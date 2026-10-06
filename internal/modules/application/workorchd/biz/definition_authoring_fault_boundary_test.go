package biz

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceowner "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 非法 typed provider 回复没有可信完成事实；持久 permit 保持未决，重放不授予第二次调用权。
func TestAuthoringMalformedTypedProviderResultRemainsUnresolved(t *testing.T) {
	ctx := context.Background()
	storage := t.TempDir()
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	hints, err := workorcheventpayload.FromMap(map[string]any{
		"profileContracts": map[string]any{
			"generic": map[string]any{
				"evidenceRequirements": []any{map[string]any{
					"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"},
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	template := workspace.WorkspaceTemplate{ID: "fault-template", DoDItems: []string{"模板验收"}, ContextHints: hints}
	if err := repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-malformed-typed-result", Revision: 1, GoalText: "目标", TemplateID: template.ID, AcceptanceRequirements: []string{"验收"}}
	if err := repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: template.ID, DeliverableDirectory: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	owner, err := workspaceowner.New(hub, nil, workspaceowner.Dependencies{OwnerRepositories: repositories.WorkspaceOwner, EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)

	var physicalCalls atomic.Int32
	provider := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	target := runtimecontract.ProviderModelTarget{ProviderID: "synthetic", ModelID: "synthetic-model"}
	if err := provider.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(llmevents.GetActiveConfigurationResult{ConfigurationRevision: "synthetic-revision", DefaultChat: target}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if err := provider.Subscribe(llmevents.TopicLLMCommandCompleteProviderDocument, func(event magicevent.Event, result magicevent.Result) {
		physicalCalls.Add(1)
		command, ok := event.Data().(llmevents.CompleteProviderDocumentCommand)
		if !ok {
			t.Errorf("provider command type %T", event.Data())
			return
		}
		if command.Request.Kind == "workspace/specification/derive" {
			row, getErr := repositories.Workspaces.Get(ctx, input.WorkspaceID)
			if getErr != nil || row.DefinitionAuthoring == nil {
				t.Errorf("derivation permit was not persisted: %v", getErr)
				return
			}
			acceptance, marshalErr := json.Marshal(workspace.DefinitionDerivedAcceptance{AuthoringInputHash: row.DefinitionAuthoring.InputHash, TemplateSnapshotHash: row.DefinitionAuthoring.TemplateSnapshotHash, AdditionalItems: []string{"派生验收"}})
			if marshalErr != nil {
				t.Error(marshalErr)
				return
			}
			result.Set(authoringTestDocumentResult(t, command, acceptance), nil)
			return
		}
		// 已观察调用但回复类型非法，Application 不得推测 provider 失败或完成。
		result.Set(struct{ Unexpected string }{Unexpected: "not a typed provider result"}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer provider.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration)
	defer provider.Unsubscribe(llmevents.TopicLLMCommandCompleteProviderDocument)

	app := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
	row, err := repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveDefinitionAuthoring(ctx, workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-malformed", Input: input, ExpectedWorkspaceRevision: row.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DeriveDefinitionAcceptance(ctx, input.WorkspaceID); err != nil {
		t.Fatalf("persist successful setup derivation: %v", err)
	}
	if _, err := app.CompileAuthoringSpecification(ctx, input.WorkspaceID); err == nil {
		t.Fatal("malformed typed result unexpectedly compiled")
	}
	view, err := app.ReadDefinitionAuthoringView(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != "outcome_unresolved" || len(view.Requests) != 1 || view.Requests[0].State != "outcome_unresolved" {
		t.Fatalf("untrusted delivery must remain unresolved: state=%q requests=%+v", view.State, view.Requests)
	}
	if _, err := app.CompileAuthoringSpecification(ctx, input.WorkspaceID); err == nil {
		t.Fatal("replay of unresolved permit unexpectedly compiled")
	}
	if physicalCalls.Load() != 2 {
		t.Fatalf("unresolved replay issued another physical call: %d", physicalCalls.Load())
	}
	stored, err := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage).Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil || stored.ActiveRunID != "" || stored.DefinitionAuthoring == nil {
		t.Fatalf("untrusted result damaged authoring state or started Run: %v", err)
	}
	requests := stored.DefinitionAuthoring.SpecificationRequests
	if len(requests) != 1 || requests[0].FailureCode != "" || requests[0].Proposal != nil || requests[0].Review != nil || requests[0].PermitID != view.Requests[0].PermitID {
		t.Fatalf("malformed reply must leave exactly the original permit unresolved: %+v", requests)
	}
}
