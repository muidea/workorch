package biz

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// AcquireDefinitionAuthoringResources asks the File owner for exact, bounded
// current file facts. A returned snapshot is not persisted authoring authority:
// before confirmation the same refs and hashes must be observed again.
func (b *Biz) AcquireDefinitionAuthoringResources(ctx context.Context, id common.WorkspaceID) ([]fileevents.AcquireAuthoringResourceResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	view, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return nil, err
	}
	if view.Record == nil || !view.CurrentTemplateAvailable || view.Record.TemplateSnapshotHash != view.CurrentTemplateSnapshotHash || view.DeliverableDirectory == "" {
		return nil, common.ErrConflict
	}
	if len(view.Record.Input.ResourceRefs) > workspace.DefinitionAuthoringResourceLimit {
		return nil, fmt.Errorf("%w: authoring resource count exceeds bounded snapshot", common.ErrInvalidArgument)
	}
	facts := make([]fileevents.AcquireAuthoringResourceResult, 0, len(view.Record.Input.ResourceRefs))
	total := 0
	for _, ref := range view.Record.Input.ResourceRefs {
		resourceFailure := func(err error) error { return &authoringResourcePreparationError{ref: ref, cause: err} }
		command := fileevents.AcquireAuthoringResourceCommand{WorkspaceID: id, AuthoringInputHash: view.Record.InputHash, ResourceRef: ref}
		if err := command.Validate(); err != nil {
			return nil, resourceFailure(err)
		}
		ev := magicevent.NewEventWithContext(fileevents.TopicAcquireAuthoringResources, ModuleID, filecommon.ModuleID, magicevent.NewHeader(), ctx, command)
		ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(fileevents.Contracts(), fileevents.TopicAcquireAuthoringResources, eventcontract.LaneScope{WorkspaceID: string(id)}))
		result := b.SendEvent(ev)
		if result == nil {
			return nil, resourceFailure(common.ErrConflict)
		}
		if err := result.Error(); err != nil {
			return nil, resourceFailure(err)
		}
		value, err := result.Get()
		if err != nil {
			return nil, resourceFailure(err)
		}
		fact, ok := value.(fileevents.AcquireAuthoringResourceResult)
		if !ok || fact.ValidateFor(command) != nil || fact.WorkspaceRevision != view.WorkspaceRevision {
			return nil, resourceFailure(common.ErrConflict)
		}
		total += len(fact.Content)
		if total > 128*1024 {
			return nil, resourceFailure(fmt.Errorf("%w: authoring resource context exceeds bounded size", common.ErrInvalidArgument))
		}
		facts = append(facts, fact)
	}
	latest, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return nil, err
	}
	if latest.Record == nil || latest.Record.InputHash != view.Record.InputHash || latest.WorkspaceRevision != view.WorkspaceRevision || !latest.CurrentTemplateAvailable || latest.CurrentTemplateSnapshotHash != view.CurrentTemplateSnapshotHash || latest.DeliverableDirectory != view.DeliverableDirectory {
		return nil, fmt.Errorf("%w: authoring input changed while observing resources", common.ErrConflict)
	}
	return facts, nil
}

// Authoring resources carry owner-issued file facts, not provider assertions.
func authoringResourceSnapshotsFor(input workspace.DefinitionAuthoringInput, facts []fileevents.AcquireAuthoringResourceResult) ([]workspace.DefinitionAuthoringResourceSnapshot, error) {
	if err := authoringResourceFactsFor(input, facts); err != nil {
		return nil, err
	}
	resources := make([]workspace.DefinitionAuthoringResourceSnapshot, 0, len(facts))
	for _, fact := range facts {
		resources = append(resources, workspace.DefinitionAuthoringResourceSnapshot{Ref: fact.ResourceRef, Content: fact.Content, ContentHash: fact.ContentHash})
	}
	if err := workspace.ValidateDefinitionAuthoringResources(input, resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func authoringResourceFactsFor(input workspace.DefinitionAuthoringInput, facts []fileevents.AcquireAuthoringResourceResult) error {
	if len(facts) != len(input.ResourceRefs) {
		return common.ErrConflict
	}
	var revision uint64
	for n, fact := range facts {
		if revision == 0 {
			revision = fact.WorkspaceRevision
		}
		if fact.WorkspaceRevision != revision || fact.WorkspaceID != input.WorkspaceID || fact.AuthoringInputHash != input.InputHash() || fact.ResourceRef != input.ResourceRefs[n] || fact.ValidateFor(fileevents.AcquireAuthoringResourceCommand{WorkspaceID: input.WorkspaceID, AuthoringInputHash: input.InputHash(), ResourceRef: input.ResourceRefs[n]}) != nil {
			return common.ErrConflict
		}
	}
	return nil
}
