package observed

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestFromEnvelopeDecisionTicketSnapshotPreservesResolutionFields(t *testing.T) {
	item, ok := FromEnvelope(events.Envelope{
		ID:      "/decision/ticket/snapshot:ticket-1",
		Version: "workorch.v1",
		Topic:   topicDecisionTicketSnapshot,
		Payload: events.DecisionResolutionPayload(
			"manual gate",
			common.DecisionTicketStateResolved,
			common.ResolutionID("resolution-1"),
			"approve-plan",
			map[string]any{"operator": "workorch"},
		),
	})
	if !ok {
		t.Fatalf("FromEnvelope returned !ok")
	}

	payload, ok := item.Payload.(DecisionTicketPayload)
	if !ok {
		t.Fatalf("payload type = %T, want DecisionTicketPayload", item.Payload)
	}
	if payload.ResolutionID != "resolution-1" {
		t.Fatalf("resolutionId = %s, want resolution-1", payload.ResolutionID)
	}
	if payload.ResolutionType != "approve-plan" {
		t.Fatalf("resolutionType = %s, want approve-plan", payload.ResolutionType)
	}
	if payload.ResolutionPayload["operator"] != "workorch" {
		t.Fatalf("resolution payload operator = %v, want workorch", payload.ResolutionPayload["operator"])
	}

	envelope := item.Envelope()
	if item.Version != events.WorkorchSnapshotVersion {
		t.Fatalf("item version = %s, want %s", item.Version, events.WorkorchSnapshotVersion)
	}
	if envelope.Version != events.WorkorchSnapshotVersion {
		t.Fatalf("envelope version = %s, want %s", envelope.Version, events.WorkorchSnapshotVersion)
	}
	if envelope.Payload[events.PayloadKeyResolutionID] != "resolution-1" {
		t.Fatalf("envelope resolutionId = %v, want resolution-1", envelope.Payload[events.PayloadKeyResolutionID])
	}
}

func TestFromEnvelopeRejectsInvalidSnapshotPayload(t *testing.T) {
	_, ok := FromEnvelope(events.Envelope{
		Topic: topicRunSnapshot,
		Payload: map[string]any{
			events.PayloadKeyState: 123,
		},
	})
	if ok {
		t.Fatalf("FromEnvelope should reject invalid snapshot payload")
	}
}

func TestFromEnvelopeRejectsIncompatibleSnapshotVersion(t *testing.T) {
	_, ok := FromEnvelope(events.Envelope{
		Version: "workorch.v2",
		Topic:   topicRunSnapshot,
		Payload: map[string]any{
			events.PayloadKeyState:              "completed",
			events.PayloadKeyBlockingTicketID:   "",
			events.PayloadKeyBlockingApprovalID: "",
		},
	})
	if ok {
		t.Fatalf("FromEnvelope should reject incompatible snapshot version")
	}
}

func TestSnapshotItemPayloadMapAndEnvelopeDeepCloneNestedPayloads(t *testing.T) {
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

	envelope := item.Envelope()
	envelopeInput := envelope.Payload[events.PayloadKeyInput].(map[string]any)
	envelopeInput["workspaceId"] = "mutated-again"
	third := item.PayloadMap()
	thirdInput := third[events.PayloadKeyInput].(map[string]any)
	if thirdInput["workspaceId"] != "ws-1" {
		t.Fatalf("workspaceId after envelope mutation = %v, want ws-1", thirdInput["workspaceId"])
	}
}
