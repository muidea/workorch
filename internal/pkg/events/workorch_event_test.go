package events

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDecodeWorkorchEventReturnsTypedRunEvent(t *testing.T) {
	event, ok := DecodeWorkorchEvent(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicRunCreated,
		RunID:   "run-1",
		Payload: map[string]any{
			PayloadKeyGoalID:             "goal-1",
			PayloadKeyDoDID:              "dod-1",
			PayloadKeyState:              string(common.RunStateRunning),
			PayloadKeyBlockingTicketID:   "",
			PayloadKeyBlockingApprovalID: "",
		},
	})
	if !ok {
		t.Fatalf("DecodeWorkorchEvent returned !ok")
	}

	runEvent, ok := event.(RunEvent)
	if !ok {
		t.Fatalf("event type = %T, want RunEvent", event)
	}
	if runEvent.Data.State != common.RunStateRunning {
		t.Fatalf("run state = %s, want %s", runEvent.Data.State, common.RunStateRunning)
	}
	if runEvent.Data.GoalID != "goal-1" || runEvent.Data.DoDID != "dod-1" {
		t.Fatalf("run goal/dod = %s/%s, want goal-1/dod-1", runEvent.Data.GoalID, runEvent.Data.DoDID)
	}
	if runEvent.Envelope().Version != WorkorchEventVersion {
		t.Fatalf("normalized version = %s, want %s", runEvent.Envelope().Version, WorkorchEventVersion)
	}
	if runEvent.Envelope().Payload[PayloadKeyState] != string(common.RunStateRunning) {
		t.Fatalf("normalized payload state = %v, want %s", runEvent.Envelope().Payload[PayloadKeyState], common.RunStateRunning)
	}
}

func TestDecodeWorkorchEventRejectsIncompatibleVersion(t *testing.T) {
	_, ok := DecodeWorkorchEvent(Envelope{
		Version: "workorch.event.v2",
		Topic:   TopicRunCreated,
		RunID:   "run-1",
		Payload: RunPayload("goal-1", "dod-1", common.RunStateRunning, "", "", ""),
	})
	if ok {
		t.Fatalf("DecodeWorkorchEvent should reject incompatible version")
	}
}

func TestSubscribeWorkorchSkipsInvalidPayload(t *testing.T) {
	hub := NewMemoryHub()
	var calls int
	unsubscribers := SubscribeWorkorch(hub, func(_ context.Context, event WorkorchEvent) error {
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
		Version: WorkorchEventVersion,
		Topic:   TopicStepStateChanged,
		StepID:  "run-1/step-1",
		Payload: map[string]any{PayloadKeyState: "completed"},
	}); err != nil {
		t.Fatalf("publish invalid step event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{
		Version:   WorkorchEventVersion,
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

func TestNormalizeWorkorchEnvelopeReturnsSanitizedCopy(t *testing.T) {
	envelope, ok := NormalizeWorkorchEnvelope(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicTicketResolved,
		Payload: map[string]any{
			PayloadKeyReason:       "manual gate",
			PayloadKeyState:        string(common.DecisionTicketStateResolved),
			PayloadKeyResolutionID: "resolution-1",
			PayloadKeyType:         "approve-plan",
			PayloadKeyPayload:      map[string]any{"operator": "workorch-demo"},
		},
	})
	if !ok {
		t.Fatalf("NormalizeWorkorchEnvelope returned !ok")
	}
	if envelope.Version != WorkorchEventVersion {
		t.Fatalf("normalized version = %s, want %s", envelope.Version, WorkorchEventVersion)
	}

	payload, ok := envelope.Payload[PayloadKeyPayload].(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map[string]any", envelope.Payload[PayloadKeyPayload])
	}
	payload["operator"] = "mutated"

	envelope2, ok := NormalizeWorkorchEnvelope(Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicTicketResolved,
		Payload: map[string]any{
			PayloadKeyReason:       "manual gate",
			PayloadKeyState:        string(common.DecisionTicketStateResolved),
			PayloadKeyResolutionID: "resolution-1",
			PayloadKeyType:         "approve-plan",
			PayloadKeyPayload:      map[string]any{"operator": "workorch-demo"},
		},
	})
	if !ok {
		t.Fatalf("second NormalizeWorkorchEnvelope returned !ok")
	}
	payload2 := envelope2.Payload[PayloadKeyPayload].(map[string]any)
	if payload2["operator"] != "workorch-demo" {
		t.Fatalf("sanitized payload operator = %v, want workorch-demo", payload2["operator"])
	}
}
