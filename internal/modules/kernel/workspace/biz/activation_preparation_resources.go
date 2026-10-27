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

func (m *Biz) observeOriginalPreparationResources(ctx context.Context, original workspaceevents.CheckWorkspacePreparationCommand) *workspace.WorkspacePreparationResourceEvidence {
	w, err := m.workspaces.Get(ctx, original.WorkspaceID)
	if err != nil {
		return nil
	}
	i, err := preparationOriginalIndex(w, original)
	if err != nil || i < 0 {
		return nil
	}
	a := w.StartupPreparation.Attempts[i]
	if a.State != workspace.WorkspacePreparationAccepted || i != len(w.StartupPreparation.Attempts)-1 || w.Revision != a.CommittedWorkspaceRevision {
		return nil
	}
	state, err := preparationStateFromPersistence(w, time.Now().UTC())
	if err != nil || state.CurrentValidity == "stale" {
		return nil
	}
	read, err := m.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil {
		return nil
	}
	s := read.Snapshot
	c := fileevents.ObservePreparationResourcesCommand{WorkspaceID: w.ID, RequestID: original.RequestID, PayloadHash: original.PayloadHash(), WorkspaceRevision: w.Revision, DeliverableDirectory: s.StartupInput.ResolvedProjectWorkspaceDirectory}
	for _, resource := range s.Candidate.Specification.Resources {
		c.ResourceRefs = append(c.ResourceRefs, resource.Ref)
	}
	if c.Validate() != nil {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ev := magicevent.NewEventWithContext(fileevents.TopicObservePreparationResources, ModuleID, "blocks/file", magicevent.NewHeader(), bounded, c)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(fileevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(w.ID)}))
	result := m.SendEvent(ev)
	if result == nil || result.Error() != nil {
		return nil
	}
	value, resultErr := result.Get()
	r, ok := value.(fileevents.ObservePreparationResourcesResult)
	if resultErr != nil || !ok || r.ValidateFor(c) != nil {
		return nil
	}
	e := workspace.WorkspacePreparationResourceEvidence{Version: workspace.PreparationResourceEvidenceVersion, WorkspaceID: w.ID, RequestID: original.RequestID, PayloadHash: original.PayloadHash(), ObservedWorkspaceRevision: w.Revision, SpecificationHash: s.AuthoringRef.SpecificationHash, DeliverableDirectory: c.DeliverableDirectory, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt}
	for i, resource := range s.Candidate.Specification.Resources {
		e.Resources = append(e.Resources, workspace.WorkspacePreparationResourceObservation{ID: resource.ID, Required: resource.Required, ExpectedContentHash: resource.SnapshotHash, Observation: r.Resources[i].Clone()})
	}
	e.ContentHash = e.Hash()
	if e.Validate() != nil || e.StartedAt.Before(a.StartedAt) || e.CompletedAt.After(time.Now().UTC()) {
		return nil
	}
	return &e
}
