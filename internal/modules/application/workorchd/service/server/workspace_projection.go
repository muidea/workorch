package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

const (
	workspaceProjectionStateReady    = workspaceviewcore.StateReady
	workspaceProjectionStateStale    = workspaceviewcore.StateStale
	workspaceProjectionStateDegraded = workspaceviewcore.StateDegraded
	workspaceProjectionStateFailed   = workspaceviewcore.StateFailed
)

func (s *serverState) workspaceProjectionStatus() projectioncontract.ProjectionStatus {
	if s == nil || s.workspaceProjection == nil {
		return projectioncontract.ProjectionStatus{State: workspaceProjectionStateFailed, Error: "workspace projection is not configured"}
	}
	return s.workspaceProjection.Status()
}

func (s *serverState) reconciledWorkspaceProjectionStatus(ctx context.Context) (projectioncontract.ProjectionStatus, error) {
	if s == nil || s.workspaceProjection == nil {
		return projectioncontract.ProjectionStatus{State: workspaceProjectionStateFailed, Error: "workspace projection is not configured"}, common.ErrConflict
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

func (s *serverState) listProjectedWorkspaces() ([]appmodelcore.Workspace, projectioncontract.ProjectionStatus, error) {
	if s == nil || s.workspaceProjection == nil {
		return nil, projectioncontract.ProjectionStatus{State: workspaceProjectionStateFailed}, common.ErrConflict
	}
	return s.workspaceProjection.ListWorkspaces()
}

func (s *HTTPServer) ListWorkspaceView(ctx context.Context) ([]appmodelcore.Workspace, error) {
	if s == nil || s.state == nil {
		return nil, common.ErrInvalidArgument
	}
	items, _, err := s.state.listProjectedWorkspaces()
	return items, err
}

func (s *serverState) readProjectedWorkspaceSnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshotState, projectioncontract.ProjectionStatus, error) {
	if s == nil || s.workspaceProjection == nil {
		return readmodelcore.WorkspaceSnapshotState{}, projectioncontract.ProjectionStatus{State: workspaceProjectionStateFailed}, common.ErrConflict
	}
	snapshot, status, err := s.workspaceProjection.ReadWorkspaceSnapshotWithContext(context.Background(), workspaceID)
	if err != nil {
		return snapshot, status, err
	}
	return s.attachSchedulerDiagnosticsToWorkspaceSnapshot(snapshot), status, nil
}

func (s *serverState) readProjectedWorkspaceSummarySnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshotState, projectioncontract.ProjectionStatus, error) {
	if s == nil || s.workspaceProjection == nil {
		return readmodelcore.WorkspaceSnapshotState{}, projectioncontract.ProjectionStatus{State: workspaceProjectionStateFailed}, common.ErrConflict
	}
	snapshot, status, err := s.workspaceProjection.ReadWorkspaceSummarySnapshot(workspaceID)
	if err != nil {
		return snapshot, status, err
	}
	return s.attachSchedulerDiagnosticsToWorkspaceSnapshot(snapshot), status, nil
}

func (s *HTTPServer) ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
	if s == nil || s.state == nil {
		return readmodelcore.WorkspaceSnapshotState{}, common.ErrInvalidArgument
	}
	snapshot, _, err := s.state.readProjectedWorkspaceSnapshot(workspaceID)
	if err != nil {
		return readmodelcore.WorkspaceSnapshotState{}, err
	}
	return readmodelprojection.WorkspaceProjection().Detail(snapshot), nil
}

func mergeWorkspaceProjectionOverlay(detail readmodelcore.WorkspaceSnapshotState, projected readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	if projected.RunCommand != nil {
		detail.RunCommand = projected.RunCommand
	}
	if projected.SchedulerDiagnostics != nil {
		detail.SchedulerDiagnostics = projected.SchedulerDiagnostics
	}
	detail = mergeWorkspaceProjectionViewModel(detail, projected.ViewModel)
	return detail
}

func mergeWorkspaceProjectionViewModel(detail readmodelcore.WorkspaceSnapshotState, projected projectioncontract.WorkspaceViewModel) readmodelcore.WorkspaceSnapshotState {
	normalized := readmodelprojection.GovernanceProjection().NormalizeWorkspace(detail)
	detail = normalized
	viewModel := projected
	if strings.TrimSpace(viewModel.WorkspaceID) == "" {
		viewModel = detail.ViewModel
	}
	if strings.TrimSpace(viewModel.WorkspaceID) == "" {
		viewModel.WorkspaceID = normalized.Workspace.ID
	}
	viewModel.PromptMode = readmodelprojection.GovernanceProjection().PromptMode(normalized)
	detail.ViewModel = viewModel
	detail.Workspace.ViewModel = workspaceMetadataViewModelFromProjection(viewModel)
	return detail
}

func (s *serverState) upsertProjectedWorkspace(workspace appmodelcore.Workspace) {
	if s == nil || s.workspaceProjectionWriter == nil {
		return
	}
	s.workspaceProjectionWriter.UpsertWorkspace(workspace)
}

func (s *serverState) deleteProjectedWorkspace(workspaceID string) {
	if s == nil || s.workspaceProjectionWriter == nil {
		return
	}
	s.workspaceProjectionWriter.DeleteWorkspace(workspaceID)
}

func (s *serverState) applyProjectedRunCommandAccepted(update workspaceviewcore.RunCommandAcceptedUpdate) bool {
	if s == nil || s.workspaceProjectionWriter == nil {
		return false
	}
	return s.workspaceProjectionWriter.ApplyRunCommandAccepted(update)
}

func (s *serverState) applyProjectedRunCommandQueued(update workspaceviewcore.RunCommandQueuedUpdate) bool {
	if s == nil || s.workspaceProjectionWriter == nil {
		return false
	}
	return s.workspaceProjectionWriter.ApplyRunCommandQueued(update)
}

func (s *serverState) applyProjectedRunCommandRejected(update workspaceviewcore.RunCommandRejectedUpdate) bool {
	if s == nil || s.workspaceProjectionWriter == nil {
		return false
	}
	return s.workspaceProjectionWriter.ApplyRunCommandRejected(update)
}

func (s *serverState) rebuildProjectedWorkspace(ctx context.Context, workspaceID string) {
	if s == nil || s.workspaceProjectionWriter == nil || workspaceID == "" {
		return
	}
	if err := s.workspaceProjectionWriter.RebuildWorkspace(ctx, workspaceID); err != nil {
		slog.Warn("workorch workspace projection refresh failed", "workspaceID", workspaceID, "err", err)
	}
}

func (s *serverState) rebuildProjectedWorkspacesForDecisions(ctx context.Context, items []projectioncontract.Decision) {
	if s == nil {
		return
	}
	if batchWriter, ok := s.workspaceProjectionWriter.(WorkspaceProjectionBatchWriter); ok {
		if err := batchWriter.RebuildWorkspacesForDecisions(ctx, items); err != nil {
			slog.Warn("workorch workspace projection decision refresh failed", "err", err)
		}
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

func (s *serverState) rebuildProjectedWorkspacesForApprovals(ctx context.Context, items []projectioncontract.Approval) {
	if s == nil {
		return
	}
	if batchWriter, ok := s.workspaceProjectionWriter.(WorkspaceProjectionBatchWriter); ok {
		if err := batchWriter.RebuildWorkspacesForApprovals(ctx, items); err != nil {
			slog.Warn("workorch workspace projection approval refresh failed", "err", err)
		}
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

func (s *HTTPServer) ReadWorkspaceActivity(ctx context.Context, workspaceID string) (projectioncontract.WorkspaceActivityFeed, bool, error) {
	if s == nil || s.state == nil {
		return projectioncontract.WorkspaceActivityFeed{}, false, common.ErrInvalidArgument
	}
	if s.state.workspaceProjection == nil {
		return projectioncontract.WorkspaceActivityFeed{}, false, common.ErrConflict
	}
	feed, ok := s.state.workspaceProjection.ReadWorkspaceActivity(workspaceID)
	return feed, ok, nil
}
