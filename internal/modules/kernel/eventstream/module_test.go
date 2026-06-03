package eventstream

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type countingSubscribeHub struct {
	subscribeCount   int
	unsubscribeCount int
}

func (h *countingSubscribeHub) Publish(context.Context, events.Envelope) error {
	return nil
}

func (h *countingSubscribeHub) Subscribe(string, events.Handler) func() {
	h.subscribeCount++
	return func() {
		h.unsubscribeCount++
	}
}

func TestModuleRunIsIdempotentAndTeardownUnsubscribes(t *testing.T) {
	ctx := context.Background()
	hub := &countingSubscribeHub{}
	module := New()
	if err := module.Configure(Dependencies{
		EventRecords: memory.NewEventRecordRepository(),
		EventHub:     hub,
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	firstSubscribeCount := hub.subscribeCount
	if firstSubscribeCount == 0 {
		t.Fatalf("subscribe count = 0, want eventstream subscriptions")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if hub.subscribeCount != firstSubscribeCount {
		t.Fatalf("subscribe count after second Run = %d, want %d", hub.subscribeCount, firstSubscribeCount)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if hub.unsubscribeCount != firstSubscribeCount {
		t.Fatalf("unsubscribe count = %d, want %d", hub.unsubscribeCount, firstSubscribeCount)
	}
}
