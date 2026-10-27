package workspace

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	filepreparation "github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// Original explicit continuation only. Re-read File after all original source
// observations; preserve both images if they differ. No token supplied by HTTP
// or Application is accepted. Replays/recovery never call this observer.
func (m *Biz) reobservePreparationFiles(ctx context.Context, d *workspace.WorkspacePreparationDirectoryEvidence, r *workspace.WorkspacePreparationResourceEvidence, c *workspace.WorkspacePreparationManagedConfigEvidence) {
	if d == nil {
		return
	}
	d.RevalidationVersion = filepreparation.Version
	d.Revalidation = nil
	defer func() { d.ContentHash = d.Hash() }()
	if r == nil || c == nil {
		return
	}
	scope := filepreparation.Scope{WorkspaceID: d.WorkspaceID, RequestID: d.RequestID, PayloadHash: d.PayloadHash, WorkspaceRevision: d.ObservedWorkspaceRevision, InternalDirectory: d.Internal.Path, DeliverableDirectory: d.Deliverable.Path}
	for _, item := range r.Resources {
		scope.ResourceRefs = append(scope.ResourceRefs, item.Observation.Ref)
	}
	for _, item := range c.Files {
		scope.Configs = append(scope.Configs, filepreparation.ConfigTarget{Scope: item.Scope, Path: item.Path})
	}
	command := fileevents.ReobservePreparationFilesCommand{Scope: scope}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ev := magicevent.NewEventWithContext(fileevents.TopicReobservePreparationFiles, ModuleID, "blocks/file", magicevent.NewHeader(), bounded, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(fileevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(scope.WorkspaceID)}))
	result := m.SendEvent(ev)
	if result == nil || result.Error() != nil {
		return
	}
	value, err := result.Get()
	observed, ok := value.(fileevents.ReobservePreparationFilesResult)
	if err != nil || !ok || observed.ValidateFor(command) != nil {
		return
	}
	o := observed.Observation.Clone()
	d.Revalidation = &o
}

func (m *Biz) preparationFileEpoch(ctx context.Context) string {
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ev := magicevent.NewEventWithContext(fileevents.TopicQueryPreparationEpoch, ModuleID, "blocks/file", magicevent.NewHeader(), bounded, fileevents.QueryPreparationEpochCommand{})
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(fileevents.Contracts(), ev.ID(), eventcontract.LaneScope{}))
	r := m.SendEvent(ev)
	if r == nil || r.Error() != nil {
		return ""
	}
	v, err := r.Get()
	value, ok := v.(fileevents.QueryPreparationEpochResult)
	if err != nil || !ok || !filepreparation.ValidEpoch(value.OwnerEpoch) {
		return ""
	}
	return value.OwnerEpoch
}
