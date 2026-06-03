package appservice

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
)

func TestCLICommandDispatcherRunIsIdempotent(t *testing.T) {
	hub := &countingCLIHub{}
	dispatcher := NewCLICommandDispatcher(&RuntimeSession{}, hub)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if hub.subscribeCount != 1 {
		t.Fatalf("subscribe count = %d, want 1", hub.subscribeCount)
	}
	dispatcher.Close()
	if hub.unsubscribeCount != 1 {
		t.Fatalf("unsubscribe count = %d, want 1", hub.unsubscribeCount)
	}
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("Run after Close: %v", err)
	}
	if hub.subscribeCount != 2 {
		t.Fatalf("subscribe count after rerun = %d, want 2", hub.subscribeCount)
	}
}

type countingCLIHub struct {
	subscribeCount   int
	unsubscribeCount int
}

func (h *countingCLIHub) Publish(context.Context, events.Envelope) error {
	return nil
}

func (h *countingCLIHub) Subscribe(string, events.Handler) func() {
	h.subscribeCount++
	return func() {
		h.unsubscribeCount++
	}
}
