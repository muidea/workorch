package observed

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
)

func TestSnapshotItemPayloadMapDeepClonesNestedPayloads(t *testing.T) {
	item := SnapshotItem{
		Topic: topicStepSnapshot,
		Payload: StepPayload{
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
	}

	first := item.PayloadMap()
	firstInput := first[events.PayloadKeyInput].(map[string]any)
	firstInput["workspaceId"] = "mutated"
	firstItems := firstInput["items"].([]any)
	firstItems[0] = "changed"
	firstItems[1].(map[string]any)["nested"] = "mutated"
	firstOutput := first[events.PayloadKeyOutput].(map[string]any)
	firstOutput["status"] = "failed"

	second := item.PayloadMap()
	secondInput := second[events.PayloadKeyInput].(map[string]any)
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
	secondOutput := second[events.PayloadKeyOutput].(map[string]any)
	if secondOutput["status"] != "completed" {
		t.Fatalf("status = %v, want completed", secondOutput["status"])
	}

}
