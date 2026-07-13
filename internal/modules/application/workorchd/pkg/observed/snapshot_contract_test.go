package observed

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
)

func TestSnapshotItemPayloadMapDeepClonesNestedPayloads(t *testing.T) {
	item := SnapshotItem{
		Topic: topicDecisionTicketSnapshot,
		Payload: DecisionTicketPayload{
			State: "resolved",
			ResolutionPayload: map[string]any{
				"workspaceId": "ws-1",
				"items":       []any{"first", map[string]any{"nested": "original"}},
			},
		},
	}

	first := item.PayloadMap()
	firstInput := first[events.PayloadKeyPayload].(map[string]any)
	firstInput["workspaceId"] = "mutated"
	firstItems := firstInput["items"].([]any)
	firstItems[0] = "changed"
	firstItems[1].(map[string]any)["nested"] = "mutated"

	second := item.PayloadMap()
	secondInput := second[events.PayloadKeyPayload].(map[string]any)
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
