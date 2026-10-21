package workspace

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) handleExactConfirmedStartupQuery(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != workspaceevents.CommandSource {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid exact startup query source"))
		return
	}
	command, ok := ev.Data().(workspaceevents.QueryExactConfirmedStartupSnapshotCommand)
	if !ok || command.Validate() != nil {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid exact startup query"))
		return
	}
	value, err := m.queryExactConfirmedStartupSnapshot(ev.Context(), command)
	if err != nil {
		// Do not echo raw persisted content or lower-level diagnostic strings.
		result.Set(nil, cd.WrapError(cd.Unexpected, err, "exact confirmed startup is unavailable"))
		return
	}
	result.Set(value, nil)
}

func (m *Biz) queryExactConfirmedStartupSnapshot(ctx context.Context, command workspaceevents.QueryExactConfirmedStartupSnapshotCommand) (workspaceevents.QueryExactConfirmedStartupSnapshotResult, error) {
	if m == nil || m.manager == nil || m.workspaces == nil || m.goals == nil || m.dods == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrInvalidArgument
	}
	m.deletionIngressMu.RLock()
	defer m.deletionIngressMu.RUnlock()
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	w, err := m.workspaces.Get(bound, command.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	if w.Revision != command.ExpectedWorkspaceRevision || workspaceDeletionRejectsMutation(w) != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrConflict
	}
	if err := validateWorkspaceAuthoringMappingOutcomes(w); err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrConflict
	}
	goal, dod, err := m.currentExecutionDefinitions(bound, w.ID)
	if err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	snapshot, err := startuppreparation.FromOwnerPersistence(w, goal, dod)
	if err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	// Metadata writers use CAS independently of the formal-definition mutex.
	// Re-read the revision; a mixed observation is never returned as exact.
	current, err := m.workspaces.Get(bound, w.ID)
	if err != nil || current.Revision != w.Revision {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrConflict
	}
	return workspaceevents.QueryExactConfirmedStartupSnapshotResult{Snapshot: snapshot}, nil
}
