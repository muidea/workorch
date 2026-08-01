package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceProjectionStatus = workspaceviewcore.Status
type workspaceProjectionMetrics = workspaceviewcore.Metrics
type workspaceProjectionManager = workspaceviewcore.Manager

const (
	workspaceProjectionStateBuilding = workspaceviewcore.StateBuilding
	workspaceProjectionStateReady    = workspaceviewcore.StateReady
	workspaceProjectionStateStale    = workspaceviewcore.StateStale
	workspaceProjectionStateDegraded = workspaceviewcore.StateDegraded
	workspaceProjectionStateFailed   = workspaceviewcore.StateFailed
	workspaceProjectionStateStopped  = workspaceviewcore.StateStopped

	workspaceProjectionRebuildCoalesceDelay = 100 * time.Millisecond
)

type workspaceProjectionLoader struct {
	state *serverState
}

func newWorkspaceProjectionManager(state *serverState) *workspaceProjectionManager {
	return workspaceviewcore.NewManager(workspaceviewcore.ManagerOptions{
		Loader:               workspaceProjectionLoader{state: state},
		Schedule:             state.scheduleBackground,
		RebuildCoalesceDelay: workspaceProjectionRebuildCoalesceDelay,
	})
}

func (l workspaceProjectionLoader) ListWorkspaces(ctx context.Context) ([]appservice.Workspace, error) {
	if l.state == nil {
		return nil, common.ErrInvalidArgument
	}
	return l.state.listWorkspaces(ctx, l.state.config.Backend, l.state.config.DataRoot)
}

func (l workspaceProjectionLoader) ReadWorkspace(ctx context.Context, workspaceID string) (appservice.Workspace, error) {
	if l.state == nil {
		return appservice.Workspace{}, common.ErrInvalidArgument
	}
	return l.state.readWorkspace(ctx, l.state.config.Backend, l.state.config.DataRoot, workspaceID)
}

func (l workspaceProjectionLoader) ReadWorkspaceSnapshot(ctx context.Context, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	if l.state == nil {
		return appservice.WorkspaceSnapshot{}, common.ErrInvalidArgument
	}
	return l.state.readProjectionSnapshot(ctx, l.state.config.Backend, l.state.config.DataRoot, workspaceID)
}

func (s *serverState) startWorkspaceProjection(ctx context.Context) error {
	if s == nil || s.workspaceProjection == nil {
		return nil
	}
	return s.workspaceProjection.Start(ctx)
}

func (s *serverState) workspaceProjectionStatus() workspaceProjectionStatus {
	if s == nil || s.workspaceProjection == nil {
		return workspaceProjectionStatus{State: workspaceProjectionStateFailed, Error: "workspace projection is not configured"}
	}
	return s.workspaceProjection.Status()
}

func (s *serverState) reconciledWorkspaceProjectionStatus(ctx context.Context) (workspaceProjectionStatus, error) {
	if s == nil || s.workspaceProjection == nil {
		return workspaceProjectionStatus{State: workspaceProjectionStateFailed, Error: "workspace projection is not configured"}, common.ErrConflict
	}
	if err := s.workspaceProjection.ReconcileSourceHighWatermarks(ctx); err != nil {
		status := s.workspaceProjection.Status()
		status.State = workspaceProjectionStateDegraded
		status.Degraded = true
		status.Error = err.Error()
		return status, err
	}
	status := s.workspaceProjection.Status()
	// A ready projection must have consumed every event that remains relevant
	// to a live workspace.  Reconciliation may have scheduled an asynchronous
	// rebuild; until that rebuild commits, health must not advertise a stale
	// read model as ready.  This is an SM-4 observation guard only: it neither
	// changes Runtime authority nor attempts to replay a Run.
	if status.SourceEventRevision != status.ProjectedSourceRevision {
		status.State = workspaceProjectionStateStale
		status.StaleReason = "source_high_watermark"
		status.Error = fmt.Sprintf("workspace projection watermark lag: source=%d projected=%d", status.SourceEventRevision, status.ProjectedSourceRevision)
		return status, fmt.Errorf("%w: %s", common.ErrConflict, status.Error)
	}
	return status, nil
}

func (s *serverState) listProjectedWorkspaces() ([]appservice.Workspace, workspaceProjectionStatus, error) {
	if s == nil || s.workspaceProjection == nil {
		return nil, workspaceProjectionStatus{State: workspaceProjectionStateFailed}, common.ErrConflict
	}
	return s.workspaceProjection.ListWorkspaces()
}

func (s *HTTPServer) ListWorkspaceView(ctx context.Context) ([]appservice.Workspace, error) {
	if s == nil || s.state == nil {
		return nil, common.ErrInvalidArgument
	}
	items, _, err := s.state.listProjectedWorkspaces()
	return items, err
}

func (s *serverState) readProjectedWorkspaceSnapshot(workspaceID string) (appservice.WorkspaceSnapshot, workspaceProjectionStatus, error) {
	if s == nil || s.workspaceProjection == nil {
		return appservice.WorkspaceSnapshot{}, workspaceProjectionStatus{State: workspaceProjectionStateFailed}, common.ErrConflict
	}
	snapshot, status, err := s.workspaceProjection.ReadWorkspaceSnapshotWithContext(context.Background(), workspaceID)
	if err != nil {
		return snapshot, status, err
	}
	return s.attachSchedulerDiagnosticsToWorkspaceSnapshot(snapshot), status, nil
}

func (s *serverState) readProjectedWorkspaceSummarySnapshot(workspaceID string) (appservice.WorkspaceSnapshot, workspaceProjectionStatus, error) {
	if s == nil || s.workspaceProjection == nil {
		return appservice.WorkspaceSnapshot{}, workspaceProjectionStatus{State: workspaceProjectionStateFailed}, common.ErrConflict
	}
	snapshot, status, err := s.workspaceProjection.ReadWorkspaceSummarySnapshot(workspaceID)
	if err != nil {
		return snapshot, status, err
	}
	return s.attachSchedulerDiagnosticsToWorkspaceSnapshot(snapshot), status, nil
}

func (s *HTTPServer) ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	if s == nil || s.state == nil {
		return appservice.WorkspaceSnapshot{}, common.ErrInvalidArgument
	}
	snapshot, _, err := s.state.readProjectedWorkspaceSnapshot(workspaceID)
	if err != nil {
		return appservice.WorkspaceSnapshot{}, err
	}
	return appservice.ProjectWorkspaceSnapshotDetail(snapshot), nil
}

func mergeWorkspaceProjectionOverlay(detail appservice.WorkspaceSnapshot, projected appservice.WorkspaceSnapshot) appservice.WorkspaceSnapshot {
	if projected.RunCommand != nil {
		detail.RunCommand = projected.RunCommand
	}
	if projected.SchedulerDiagnostics != nil {
		detail.SchedulerDiagnostics = projected.SchedulerDiagnostics
	}
	detail = mergeWorkspaceProjectionViewModel(detail, projected.ViewModel)
	return detail
}

func mergeWorkspaceProjectionViewModel(detail appservice.WorkspaceSnapshot, projected readmodelcore.WorkspaceViewModel) appservice.WorkspaceSnapshot {
	normalized := readmodelprojection.NormalizeWorkspaceGovernanceProjection(detail)
	detail = normalized
	viewModel := projected
	if strings.TrimSpace(viewModel.WorkspaceID) == "" {
		viewModel = detail.ViewModel
	}
	if strings.TrimSpace(viewModel.WorkspaceID) == "" {
		viewModel.WorkspaceID = normalized.Workspace.ID
	}
	viewModel.PromptMode = readmodelprojection.WorkspacePromptMode(normalized)
	detail.ViewModel = viewModel
	detail.Workspace.ViewModel = workspaceMetadataViewModelFromProjection(viewModel)
	return detail
}

func (s *serverState) upsertProjectedWorkspace(workspace appservice.Workspace) {
	if s == nil || s.workspaceProjection == nil {
		return
	}
	s.workspaceProjection.UpsertWorkspace(workspace)
}

func (s *serverState) rebuildProjectedWorkspace(ctx context.Context, workspaceID string) {
	if s == nil || s.workspaceProjection == nil || workspaceID == "" {
		return
	}
	if err := s.workspaceProjection.RebuildWorkspace(ctx, workspaceID); err != nil {
		slog.Warn("workorch workspace projection refresh failed", "workspaceID", workspaceID, "err", err)
	}
}

func (s *serverState) rebuildProjectedWorkspacesForDecisions(ctx context.Context, items []appservice.Decision) {
	if s == nil {
		return
	}
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.WorkspaceID == "" {
			continue
		}
		if _, ok := seen[item.WorkspaceID]; ok {
			continue
		}
		seen[item.WorkspaceID] = struct{}{}
		s.rebuildProjectedWorkspace(ctx, item.WorkspaceID)
	}
}

func (s *serverState) rebuildProjectedWorkspacesForApprovals(ctx context.Context, items []appservice.Approval) {
	if s == nil {
		return
	}
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.WorkspaceID == "" {
			continue
		}
		if _, ok := seen[item.WorkspaceID]; ok {
			continue
		}
		seen[item.WorkspaceID] = struct{}{}
		s.rebuildProjectedWorkspace(ctx, item.WorkspaceID)
	}
}

func (s *HTTPServer) ReadWorkspaceActivity(ctx context.Context, workspaceID string) (appservice.WorkspaceActivityFeed, bool, error) {
	if s == nil || s.state == nil {
		return appservice.WorkspaceActivityFeed{}, false, common.ErrInvalidArgument
	}
	if s.state.workspaceProjection == nil {
		return appservice.WorkspaceActivityFeed{}, false, common.ErrConflict
	}
	feed, ok := s.state.workspaceProjection.ReadWorkspaceActivity(workspaceID)
	return feed, ok, nil
}
