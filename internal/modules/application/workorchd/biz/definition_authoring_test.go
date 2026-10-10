package biz

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestAuthoringApplicationRejectsMismatchedOwnerResults(t *testing.T) {
	for _, scenario := range []string{"valid", "scope", "receipt_revision", "duplicate_binding"} {
		mismatch := scenario == "scope"
		ctx := context.Background()
		hub := magicevent.NewHub(32)
		b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
		owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
		input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
		template := workspace.WorkspaceTemplate{ID: "t", SoulBindings: map[string]string{"planning": "planner"}}
		record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
		if mismatch {
			record.Input.WorkspaceID = "other"
			record.InputHash = record.Input.InputHash()
		}
		if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave, func(ev magicevent.Event, result magicevent.Result) {
			command := ev.Data().(workspaceevents.SaveDefinitionAuthoringCommand)
			receipt := workspace.DefinitionAuthoringSaveReceipt{RequestID: command.RequestID, RequestHash: workspace.DefinitionAuthoringSaveRequestHash(command.RequestID, command.Input, command.ExpectedWorkspaceRevision, command.ExpectedInputHash, command.DeliverableDirectory), InputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, WorkspaceRevision: 2}
			if mismatch {
				receipt.RequestHash = "wrong"
			}
			if scenario == "receipt_revision" {
				receipt.WorkspaceRevision++
			}
			record.SaveReceipts = []workspace.DefinitionAuthoringSaveReceipt{receipt}
			dto := authoringTestRecord(record)
			if scenario == "duplicate_binding" {
				dto.TemplateSnapshot.SoulBindings = append(dto.TemplateSnapshot.SoulBindings, dto.TemplateSnapshot.SoulBindings[0])
			}
			result.Set(workspaceevents.SaveDefinitionAuthoringResult{Record: *dto, Receipt: receipt}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, func(ev magicevent.Event, result magicevent.Result) {
			dto := authoringTestRecord(record)
			if scenario == "duplicate_binding" {
				dto.TemplateSnapshot.SoulBindings = append(dto.TemplateSnapshot.SoulBindings, dto.TemplateSnapshot.SoulBindings[0])
			}
			result.Set(workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: "ws", WorkspaceRevision: 1, Record: dto, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash, CurrentTemplateAvailable: true}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		_, saveErr := b.SaveDefinitionAuthoring(ctx, workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-test", Input: input, ExpectedWorkspaceRevision: 1})
		_, queryErr := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: "ws"})
		owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave)
		owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery)
		hub.Terminate(ctx)
		if (saveErr != nil) != (scenario != "valid") || (queryErr != nil) != (mismatch || scenario == "duplicate_binding") {
			t.Fatalf("mismatch=%v save=%v query=%v", mismatch, saveErr, queryErr)
		}
	}
}
