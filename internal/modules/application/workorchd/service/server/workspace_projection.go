package server

import (
	"context"
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
		Hub:                  state.eventHub,
		Schedule:             state.scheduleBackground,
		RebuildCoalesceDelay: workspaceProjectionRebuildCoalesceDelay,
	})
}

func (l workspaceProjectionLoader) ListWorkspaces(ctx context.Context) ([]appservice.Workspace, error) {
	if l.state == nil {
		return nil, common.ErrInvalidArgument
	}
	ctx = appservice.ContextWithEventHubs(ctx, l.state.eventHub, l.state.commandHub)
	return l.state.listWorkspaces(ctx, l.state.config.Backend, l.state.config.DataRoot)
}

func (l workspaceProjectionLoader) ReadWorkspace(ctx context.Context, workspaceID string) (appservice.Workspace, error) {
	if l.state == nil {
		return appservice.Workspace{}, common.ErrInvalidArgument
	}
	ctx = appservice.ContextWithEventHubs(ctx, l.state.eventHub, l.state.commandHub)
	return l.state.readWorkspace(ctx, l.state.config.Backend, l.state.config.DataRoot, workspaceID)
}

func (l workspaceProjectionLoader) ReadWorkspaceSnapshot(ctx context.Context, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	if l.state == nil {
		return appservice.WorkspaceSnapshot{}, common.ErrInvalidArgument
	}
	ctx = appservice.ContextWithEventHubs(ctx, l.state.eventHub, l.state.commandHub)
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
	return s.workspaceProjection.ReadWorkspaceSnapshotWithContext(context.Background(), workspaceID)
}

func (s *serverState) readProjectedWorkspaceSummarySnapshot(workspaceID string) (appservice.WorkspaceSnapshot, workspaceProjectionStatus, error) {
	if s == nil || s.workspaceProjection == nil {
		return appservice.WorkspaceSnapshot{}, workspaceProjectionStatus{State: workspaceProjectionStateFailed}, common.ErrConflict
	}
	return s.workspaceProjection.ReadWorkspaceSummarySnapshot(workspaceID)
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
