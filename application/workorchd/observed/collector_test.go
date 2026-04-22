package observed

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
)

func TestCollectorCollectsSubscribedPhase1Events(t *testing.T) {
	hub := events.NewMemoryHub()
	collector := NewCollector(hub)
	defer collector.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.Phase1EventVersion,
		Topic:       events.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-1",
		RunID:       "run-1",
		Payload:     map[string]any{events.PayloadKeyState: "running"},
	}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}
	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       "/unrelated/topic",
		SubjectType: "other",
		SubjectID:   "other-1",
		Payload:     map[string]any{events.PayloadKeyState: "ignored"},
	}); err != nil {
		t.Fatalf("publish unrelated event: %v", err)
	}

	published := collector.PublishedPhase1Events()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic() != events.TopicRunStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic(), events.TopicRunStateChanged)
	}
	if published[0].Envelope().Payload[events.PayloadKeyState] != "running" {
		t.Fatalf("payload state = %v, want running", published[0].Envelope().Payload[events.PayloadKeyState])
	}
}

func TestCollectorCloseStopsFurtherCollection(t *testing.T) {
	hub := events.NewMemoryHub()
	collector := NewCollector(hub)
	collector.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.Phase1EventVersion,
		Topic:       events.TopicApprovalCreated,
		SubjectType: "approval",
		SubjectID:   "approval-1",
		Payload:     map[string]any{events.PayloadKeyState: "pending"},
	}); err != nil {
		t.Fatalf("publish approval event: %v", err)
	}

	if published := collector.PublishedPhase1Events(); len(published) != 0 {
		t.Fatalf("len(published) = %d, want 0", len(published))
	}
}

func TestCollectorSkipsInvalidPhase1Payload(t *testing.T) {
	hub := events.NewMemoryHub()
	collector := NewCollector(hub)
	defer collector.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.Phase1EventVersion,
		Topic:       events.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-1",
		RunID:       "run-1",
		Payload: map[string]any{
			events.PayloadKeyState: 123,
		},
	}); err != nil {
		t.Fatalf("publish invalid run event: %v", err)
	}

	if published := collector.PublishedPhase1Events(); len(published) != 0 {
		t.Fatalf("len(published) = %d, want 0", len(published))
	}
}
