package biz

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringApplicationRejectsMismatchedOwnerResults(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		ctx := context.Background()
		hub := magicevent.NewHub(32)
		b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
		owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
		input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
		template := workspace.WorkspaceTemplate{ID: "t"}
		record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
		if mismatch {
			record.Input.WorkspaceID = "other"
			record.InputHash = record.Input.InputHash()
		}
		if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave, func(ev magicevent.Event, result magicevent.Result) {
			result.Set(workspaceevents.SaveDefinitionAuthoringResult{Record: record}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, func(ev magicevent.Event, result magicevent.Result) {
			result.Set(workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: "ws", WorkspaceRevision: 1, Record: &record, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash, CurrentTemplateAvailable: true}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		_, saveErr := b.SaveDefinitionAuthoring(ctx, workspaceevents.SaveDefinitionAuthoringCommand{Input: input, ExpectedWorkspaceRevision: 1})
		_, queryErr := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: "ws"})
		owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave)
		owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery)
		hub.Terminate(ctx)
		if (saveErr != nil) != mismatch || (queryErr != nil) != mismatch {
			t.Fatalf("mismatch=%v save=%v query=%v", mismatch, saveErr, queryErr)
		}
	}
}
