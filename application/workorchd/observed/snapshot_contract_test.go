package observed

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestFromEnvelopeDecisionTicketSnapshotPreservesResolutionFields(t *testing.T) {
	item, ok := FromEnvelope(events.Envelope{
		ID:      "/decision/ticket/snapshot:ticket-1",
		Version: "phase1-demo.v1",
		Topic:   topicDecisionTicketSnapshot,
		Payload: events.DecisionResolutionPayload(
			"manual gate",
			common.DecisionTicketStateResolved,
			common.ResolutionID("resolution-1"),
			"approve-plan",
			map[string]any{"operator": "phase1-demo"},
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
	if payload.ResolutionPayload["operator"] != "phase1-demo" {
		t.Fatalf("resolution payload operator = %v, want phase1-demo", payload.ResolutionPayload["operator"])
	}

	envelope := item.Envelope()
	if item.Version != events.Phase1SnapshotVersion {
		t.Fatalf("item version = %s, want %s", item.Version, events.Phase1SnapshotVersion)
	}
	if envelope.Version != events.Phase1SnapshotVersion {
		t.Fatalf("envelope version = %s, want %s", envelope.Version, events.Phase1SnapshotVersion)
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
		Version: "phase1-demo.v2",
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
