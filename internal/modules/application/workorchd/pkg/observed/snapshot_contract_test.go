package observed

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustObservedPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestSnapshotItemPayloadMapDeepClonesNestedPayloads(t *testing.T) {
	item := SnapshotItem{
		Topic: topicDecisionTicketSnapshot,
		Payload: DecisionTicketPayload{
			State: "resolved",
			ResolutionPayload: mustObservedPayload(t, map[string]any{
				"workspaceId": "ws-1",
				"items":       []any{"first", map[string]any{"nested": "original"}},
			}),
		},
	}

	first := item.PayloadMap()
	firstInput := first[recordpayload.PayloadKeyPayload].(map[string]any)
	firstInput["workspaceId"] = "mutated"
	firstItems := firstInput["items"].([]any)
	firstItems[0] = "changed"
	firstItems[1].(map[string]any)["nested"] = "mutated"

	second := item.PayloadMap()
	secondInput := second[recordpayload.PayloadKeyPayload].(map[string]any)
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
