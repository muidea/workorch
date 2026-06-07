package runtimeevent

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestExecutorRunIsIdempotent(t *testing.T) {
	hub := &countingRuntimeHub{}
	executor := NewExecutor("test-block", hub, noopRuntimeAdapter{}, "test/kind")
	if err := executor.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := executor.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if hub.subscribeCount != 1 {
		t.Fatalf("subscribe count = %d, want 1", hub.subscribeCount)
	}
	executor.Teardown()
	if hub.unsubscribeCount != 1 {
		t.Fatalf("unsubscribe count = %d, want 1", hub.unsubscribeCount)
	}
	if err := executor.Run(); err != nil {
		t.Fatalf("Run after Teardown: %v", err)
	}
	if hub.subscribeCount != 2 {
		t.Fatalf("subscribe count after rerun = %d, want 2", hub.subscribeCount)
	}
}

type noopRuntimeAdapter struct{}

func (noopRuntimeAdapter) ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return runtimemodel.StepExecutionResult{}, nil
}

type countingRuntimeHub struct {
	subscribeCount   int
	unsubscribeCount int
}

func (h *countingRuntimeHub) Publish(context.Context, events.Envelope) error {
	return nil
}

func (h *countingRuntimeHub) Subscribe(string, events.Handler) func() {
	h.subscribeCount++
	return func() {
		h.unsubscribeCount++
	}
}
