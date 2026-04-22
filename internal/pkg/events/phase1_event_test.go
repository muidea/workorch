package events

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDecodePhase1EventReturnsTypedRunEvent(t *testing.T) {
	event, ok := DecodePhase1Event(Envelope{
		Version: Phase1EventVersion,
		Topic:   TopicRunStateChanged,
		RunID:   "run-1",
		Payload: map[string]any{
			PayloadKeyState:              string(common.RunStateRunning),
			PayloadKeyBlockingTicketID:   "",
			PayloadKeyBlockingApprovalID: "",
		},
	})
	if !ok {
		t.Fatalf("DecodePhase1Event returned !ok")
	}

	runEvent, ok := event.(RunEvent)
	if !ok {
		t.Fatalf("event type = %T, want RunEvent", event)
	}
	if runEvent.Data.State != common.RunStateRunning {
		t.Fatalf("run state = %s, want %s", runEvent.Data.State, common.RunStateRunning)
	}
	if runEvent.Envelope().Version != Phase1EventVersion {
		t.Fatalf("normalized version = %s, want %s", runEvent.Envelope().Version, Phase1EventVersion)
	}
	if runEvent.Envelope().Payload[PayloadKeyState] != string(common.RunStateRunning) {
		t.Fatalf("normalized payload state = %v, want %s", runEvent.Envelope().Payload[PayloadKeyState], common.RunStateRunning)
	}
}

func TestDecodePhase1EventRejectsIncompatibleVersion(t *testing.T) {
	_, ok := DecodePhase1Event(Envelope{
		Version: "phase1.event.v2",
		Topic:   TopicRunStateChanged,
		RunID:   "run-1",
		Payload: RunPayload(common.RunStateRunning, "", ""),
	})
	if ok {
		t.Fatalf("DecodePhase1Event should reject incompatible version")
	}
}

func TestSubscribePhase1SkipsInvalidPayload(t *testing.T) {
	hub := NewMemoryHub()
	var calls int
	unsubscribers := SubscribePhase1(hub, func(_ context.Context, event Phase1Event) error {
		calls++
		return nil
	})
	defer func() {
		for _, unsubscribe := range unsubscribers {
			if unsubscribe != nil {
				unsubscribe()
			}
		}
	}()

	if err := hub.Publish(context.Background(), Envelope{
		Version: Phase1EventVersion,
		Topic:   TopicStepStateChanged,
		StepID:  "run-1/step-1",
		Payload: map[string]any{PayloadKeyState: "completed"},
	}); err != nil {
		t.Fatalf("publish invalid step event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{
		Version:   Phase1EventVersion,
		Topic:     TopicApprovalResolved,
		RunID:     "run-1",
		SubjectID: "approval-1",
		Payload:   ApprovalPayload("gate", common.ApprovalStateApproved),
	}); err != nil {
		t.Fatalf("publish valid approval event: %v", err)
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestNormalizePhase1EnvelopeReturnsSanitizedCopy(t *testing.T) {
	envelope, ok := NormalizePhase1Envelope(Envelope{
		Version: Phase1EventVersion,
		Topic:   TopicTicketResolved,
		Payload: map[string]any{
			PayloadKeyReason:       "manual gate",
			PayloadKeyState:        string(common.DecisionTicketStateResolved),
			PayloadKeyResolutionID: "resolution-1",
			PayloadKeyType:         "approve-plan",
			PayloadKeyPayload:      map[string]any{"operator": "phase1-demo"},
		},
	})
	if !ok {
		t.Fatalf("NormalizePhase1Envelope returned !ok")
	}
	if envelope.Version != Phase1EventVersion {
		t.Fatalf("normalized version = %s, want %s", envelope.Version, Phase1EventVersion)
	}

	payload, ok := envelope.Payload[PayloadKeyPayload].(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map[string]any", envelope.Payload[PayloadKeyPayload])
	}
	payload["operator"] = "mutated"

	envelope2, ok := NormalizePhase1Envelope(Envelope{
		Version: Phase1EventVersion,
		Topic:   TopicTicketResolved,
		Payload: map[string]any{
			PayloadKeyReason:       "manual gate",
			PayloadKeyState:        string(common.DecisionTicketStateResolved),
			PayloadKeyResolutionID: "resolution-1",
			PayloadKeyType:         "approve-plan",
			PayloadKeyPayload:      map[string]any{"operator": "phase1-demo"},
		},
	})
	if !ok {
		t.Fatalf("second NormalizePhase1Envelope returned !ok")
	}
	payload2 := envelope2.Payload[PayloadKeyPayload].(map[string]any)
	if payload2["operator"] != "phase1-demo" {
		t.Fatalf("sanitized payload operator = %v, want phase1-demo", payload2["operator"])
	}
}
