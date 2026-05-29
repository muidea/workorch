package presenter

import (
	"testing"

	workorchdcommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

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
			ID:      "/step/snapshot:step-1",
			Version: "workorch.v1",
			Topic:   "/step/snapshot",
			Payload: observed.StepPayload{
				Kind:  "llm/workspace_execution",
				State: "completed",
				Input: map[string]any{
					"workspaceId": "ws-1",
					"items":       []any{"first", map[string]any{"nested": "original"}},
				},
				Output: map[string]any{
					"status": "completed",
				},
			},
		},
	}

	first := MapObserved(items)
	firstInput := first[0].Payload[events.PayloadKeyInput].(map[string]any)
	firstInput["workspaceId"] = "mutated"
	firstItems := firstInput["items"].([]any)
	firstItems[0] = "changed"
	firstItems[1].(map[string]any)["nested"] = "mutated"
	firstOutput := first[0].Payload[events.PayloadKeyOutput].(map[string]any)
	firstOutput["status"] = "failed"

	sourcePayload := items[0].Payload.(observed.StepPayload)
	if sourcePayload.Input["workspaceId"] != "ws-1" {
		t.Fatalf("source workspaceId = %v, want ws-1", sourcePayload.Input["workspaceId"])
	}
	sourceItems := sourcePayload.Input["items"].([]any)
	if sourceItems[0] != "first" {
		t.Fatalf("source items[0] = %v, want first", sourceItems[0])
	}
	if sourceItems[1].(map[string]any)["nested"] != "original" {
		t.Fatalf("source nested = %v, want original", sourceItems[1].(map[string]any)["nested"])
	}
	if sourcePayload.Output["status"] != "completed" {
		t.Fatalf("source status = %v, want completed", sourcePayload.Output["status"])
	}

	second := MapObserved(items)
	secondInput := second[0].Payload[events.PayloadKeyInput].(map[string]any)
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
	secondOutput := second[0].Payload[events.PayloadKeyOutput].(map[string]any)
	if secondOutput["status"] != "completed" {
		t.Fatalf("status = %v, want completed", secondOutput["status"])
	}
}

func TestMapStepDeepClonesNestedPayloads(t *testing.T) {
	step := &runmodel.Step{
		ID:    "step-1",
		Kind:  "llm/workspace_execution",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"workspaceId": "ws-1",
			"items":       []any{"first", map[string]any{"nested": "original"}},
		},
		Output: map[string]any{
			"result": map[string]any{"status": "completed"},
		},
	}

	mapped := MapStep(step)
	mapped.Input["workspaceId"] = "mutated"
	mappedItems := mapped.Input["items"].([]any)
	mappedItems[0] = "changed"
	mappedItems[1].(map[string]any)["nested"] = "mutated"
	mapped.Output["result"].(map[string]any)["status"] = "failed"

	if step.Input["workspaceId"] != "ws-1" {
		t.Fatalf("source workspaceId = %v, want ws-1", step.Input["workspaceId"])
	}
	sourceItems := step.Input["items"].([]any)
	if sourceItems[0] != "first" {
		t.Fatalf("source items[0] = %v, want first", sourceItems[0])
	}
	if sourceItems[1].(map[string]any)["nested"] != "original" {
		t.Fatalf("source nested = %v, want original", sourceItems[1].(map[string]any)["nested"])
	}
	if step.Output["result"].(map[string]any)["status"] != "completed" {
		t.Fatalf("source status = %v, want completed", step.Output["result"].(map[string]any)["status"])
	}
}

func TestMapStepCanonicalizesWorkspacePlanKind(t *testing.T) {
	step := &runmodel.Step{
		ID:    "step-alias",
		Kind:  llmcommon.StepKindWorkspacePlan,
		State: common.StepStateCompleted,
	}

	mapped := MapStep(step)

	if mapped.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("mapped.Kind = %q, want %q", mapped.Kind, llmcommon.StepKindWorkspaceExecution)
	}
}
