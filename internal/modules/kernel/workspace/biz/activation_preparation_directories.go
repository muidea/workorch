package workspace

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// Owner verification for the application's original settlement. The public
// command supplies neither paths nor evidence. Resolve the exact current seal
// before asking File, outside reducer/definition locks. The final CAS fences
// revision/epochs again; this historical read is never an atomic FS promise.
func (m *Biz) observeOriginalPreparationDirectories(ctx context.Context, original workspaceevents.CheckWorkspacePreparationCommand) (*workspace.WorkspacePreparationDirectoryEvidence, string) {
	w, err := m.workspaces.Get(ctx, original.WorkspaceID)
	if err != nil {
		return nil, "directory_observation_unavailable"
	}
	i, err := preparationOriginalIndex(w, original)
	if err != nil || i < 0 {
		return nil, "directory_observation_unavailable"
	}
	a := w.StartupPreparation.Attempts[i]
	// A duplicate/late continuation is readback only. Explicit recovery never
	// calls this method. No completed original may trigger another owner read.
	if a.State != workspace.WorkspacePreparationAccepted || i != len(w.StartupPreparation.Attempts)-1 || w.Revision != a.CommittedWorkspaceRevision {
		return nil, "directory_observation_unavailable"
	}
	state, err := preparationStateFromPersistence(w, time.Now().UTC())
	if err != nil || state.CurrentValidity == "stale" {
		return nil, "directory_observation_unavailable"
	}
	read, err := m.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil {
		return nil, "directory_observation_unavailable"
	}
	s := read.Snapshot
	c := fileevents.ObservePreparationDirectoriesCommand{WorkspaceID: w.ID, RequestID: original.RequestID, PayloadHash: original.PayloadHash(), WorkspaceRevision: w.Revision, InternalDirectory: s.InternalDirectory, DeliverableDirectory: s.StartupInput.ResolvedProjectWorkspaceDirectory}
	if c.Validate() != nil {
		return nil, "directory_observation_unavailable"
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ev := magicevent.NewEventWithContext(fileevents.TopicObservePreparationDirectories, ModuleID, "blocks/file", magicevent.NewHeader(), bounded, c)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(fileevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(w.ID)}))
	result := m.SendEvent(ev)
	if result == nil || result.Error() != nil {
		return nil, "directory_observation_unavailable"
	}
	value, resultErr := result.Get()
	r, ok := value.(fileevents.ObservePreparationDirectoriesResult)
	if resultErr != nil || !ok || r.ValidateFor(c) != nil {
		return nil, "directory_observation_unavailable"
	}
	e := workspace.WorkspacePreparationDirectoryEvidence{Version: workspace.PreparationDirectoryEvidenceVersion, WorkspaceID: w.ID, RequestID: original.RequestID, PayloadHash: original.PayloadHash(), ObservedWorkspaceRevision: w.Revision, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, Internal: r.Internal.Clone(), Deliverable: r.Deliverable.Clone()}
	e.ContentHash = e.Hash()
	if e.Validate() != nil || r.StartedAt.Before(a.StartedAt) || r.CompletedAt.After(time.Now().UTC()) {
		return nil, "directory_observation_unavailable"
	}
	return &e, e.BlockedReason()
}
