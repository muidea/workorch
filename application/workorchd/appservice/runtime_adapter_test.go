package appservice

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
)

type testHub struct {
	subscribers map[string][]events.Handler
}

func newTestHub() *testHub {
	return &testHub{subscribers: map[string][]events.Handler{}}
}

func (h *testHub) Publish(ctx context.Context, envelope events.Envelope) error {
	for _, handler := range h.subscribers[envelope.Topic] {
		if handler == nil {
			continue
		}
		if err := handler(ctx, envelope); err != nil {
			return err
		}
	}
	return nil
}

func (h *testHub) Subscribe(topic string, handler events.Handler) func() {
	h.subscribers[topic] = append(h.subscribers[topic], handler)
	return func() {}
}

func TestManagedRuntimePublishedPhase1EventsUsesCollectorRatherThanHubPublishedMethod(t *testing.T) {
	hub := newTestHub()
	runtime := NewManagedRuntime(NewRuntimeAdapter(RuntimeDependencies{}), hub)
	defer runtime.Close()

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

	published := runtime.PublishedPhase1Events()
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

func TestManagedRuntimeCloseStopsFurtherCollection(t *testing.T) {
	hub := newTestHub()
	runtime := NewManagedRuntime(NewRuntimeAdapter(RuntimeDependencies{}), hub)

	runtime.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.Phase1EventVersion,
		Topic:       events.TopicApprovalCreated,
		SubjectType: "approval",
		SubjectID:   "approval-1",
		Payload:     map[string]any{events.PayloadKeyState: "pending"},
	}); err != nil {
		t.Fatalf("publish approval event: %v", err)
	}

	if published := runtime.PublishedPhase1Events(); len(published) != 0 {
		t.Fatalf("len(published) = %d, want 0", len(published))
	}
}
