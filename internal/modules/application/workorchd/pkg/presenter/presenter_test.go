package presenter

import (
	"testing"

	workorchdcommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustPresenterStepPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestMapObservedPreservesEnvelopeShape(t *testing.T) {
	items := []observed.SnapshotItem{
		{
			ID:           "/run/snapshot:run-1",
			Version:      "workorch.v1",
			Topic:        "/run/snapshot",
			OccurredAt:   "2026-04-22T00:00:00Z",
			SourceModule: workorchdcommon.EventSourceModuleID,
			SubjectType:  "run",
			SubjectID:    "run-1",
			WorkspaceID:  "ws-1",
			RunID:        "run-1",
			Payload: observed.RunPayload{
				State:              "completed",
				BlockingTicketID:   "",
				BlockingApprovalID: "",
			},
		},
	}

	mapped := MapObserved(items)
	if len(mapped) != 1 {
		t.Fatalf("len(mapped) = %d, want 1", len(mapped))
	}
	if mapped[0].Topic != "/run/snapshot" {
		t.Fatalf("topic = %s, want /run/snapshot", mapped[0].Topic)
	}
	if mapped[0].Payload["state"] != "completed" {
		t.Fatalf("payload state = %v, want completed", mapped[0].Payload["state"])
	}
}

func TestMapObservedDeepClonesNestedPayloads(t *testing.T) {
	items := []observed.SnapshotItem{
		{
			ID:      "/decision/ticket/snapshot:ticket-1",
			Version: "workorch.v1",
			Topic:   "/decision/ticket/snapshot",
			Payload: observed.DecisionTicketPayload{
				State: "resolved",
				ResolutionPayload: workorcheventpayload.New(
					workorcheventpayload.StringField("workspaceId", "ws-1"),
					workorcheventpayload.Field{Name: "items", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: []workorcheventpayload.Value{
						{Kind: workorcheventpayload.ValueString, String: "first"},
						{Kind: workorcheventpayload.ValueObject, Object: []workorcheventpayload.Field{workorcheventpayload.StringField("nested", "original")}},
					}}},
				),
			},
		},
	}

	first := MapObserved(items)
	firstInput := first[0].Payload[events.PayloadKeyPayload].(map[string]any)
	firstInput["workspaceId"] = "mutated"
	firstItems := firstInput["items"].([]any)
	firstItems[0] = "changed"
	firstItems[1].(map[string]any)["nested"] = "mutated"
	sourcePayload := items[0].Payload.(observed.DecisionTicketPayload)
	workspaceID, _ := sourcePayload.ResolutionPayload.String("workspaceId")
	if workspaceID != "ws-1" {
		t.Fatalf("source workspaceId = %v, want ws-1", workspaceID)
	}
	sourceItems, _ := sourcePayload.ResolutionPayload.Field("items")
	if sourceItems.Array[0].String != "first" {
		t.Fatalf("source items[0] = %v, want first", sourceItems.Array[0])
	}
	nested, _ := (workorcheventpayload.Payload{Fields: sourceItems.Array[1].Object}).String("nested")
	if nested != "original" {
		t.Fatalf("source nested = %v, want original", nested)
	}
	second := MapObserved(items)
	secondInput := second[0].Payload[events.PayloadKeyPayload].(map[string]any)
	if secondInput["workspaceId"] != "ws-1" {
		t.Fatalf("workspaceId = %v, want ws-1", secondInput["workspaceId"])
	}
	secondItems := secondInput["items"].([]any)
	if secondItems[0] != "first" {
		t.Fatalf("items[0] = %v, want first", secondItems[0])
	}
	if secondItems[1].(map[string]any)["nested"] != "original" {
		t.Fatalf("nested = %v, want original", secondItems[1].(map[string]any)["nested"])
	}
}

func TestMapStepDeepClonesNestedPayloads(t *testing.T) {
	step := &runmodel.Step{
		ID:    "step-1",
		Kind:  "llm/workspace_execution",
		State: common.StepStateSucceeded,
		Input: mustPresenterStepPayload(t, map[string]any{
			"workspaceId": "ws-1",
			"items":       []any{"first", map[string]any{"nested": "original"}},
		}),
		Output: mustPresenterStepPayload(t, map[string]any{
			"result": map[string]any{"status": "completed"},
		}),
	}

	mapped := MapStep(step)
	mapped.Input["workspaceId"] = "mutated"
	mappedItems := mapped.Input["items"].([]any)
	mappedItems[0] = "changed"
	mappedItems[1].(map[string]any)["nested"] = "mutated"
	mapped.Output["result"].(map[string]any)["status"] = "failed"

	workspaceID, _ := step.Input.String("workspaceId")
	if workspaceID != "ws-1" {
		t.Fatalf("source workspaceId = %v, want ws-1", workspaceID)
	}
	itemsValue, _ := step.Input.Field("items")
	if len(itemsValue.Array) != 2 || itemsValue.Array[0].String != "first" {
		t.Fatalf("source items = %+v, want first", itemsValue.Array)
	}
	nested := workorcheventpayload.Payload{Fields: itemsValue.Array[1].Object}
	nestedValue, _ := nested.String("nested")
	if nestedValue != "original" {
		t.Fatalf("source nested = %v, want original", nestedValue)
	}
	resultPayload, _ := step.Output.Object("result")
	status, _ := resultPayload.String("status")
	if status != "completed" {
		t.Fatalf("source status = %v, want completed", status)
	}
}

func TestMapStepCanonicalizesWorkspacePlanKind(t *testing.T) {
	step := &runmodel.Step{
		ID:    "step-alias",
		Kind:  llmcommon.StepKindWorkspacePlan,
		State: common.StepStateSucceeded,
	}

	mapped := MapStep(step)

	if mapped.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("mapped.Kind = %q, want %q", mapped.Kind, llmcommon.StepKindWorkspaceExecution)
	}
}
