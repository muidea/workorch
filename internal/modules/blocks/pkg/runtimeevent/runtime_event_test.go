package runtimeevent

import (
	"context"
	"testing"
	"time"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

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

func TestExecutorRuntimeBlockCommandDoesNotWaitForRunStateLane(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())

	executor := NewExecutor("test-block", hub, noopRuntimeAdapter{}, "mcp/tool_call")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	unsubscribeWrite := hub.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(_ context.Context, envelope events.Envelope) error {
		if envelope.RunID != "run-001" {
			t.Fatalf("write run ID = %q, want run-001", envelope.RunID)
		}
		close(writeStarted)
		<-releaseWrite
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response"})
	})
	defer unsubscribeWrite()

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- hub.Publish(context.Background(), events.Envelope{
			Topic:       orchestratorevents.TopicOrchestratorCommandResumeRun,
			WorkspaceID: "ws-a",
			RunID:       "run-001",
		})
	}()

	select {
	case <-writeStarted:
	case <-time.After(time.Second):
		t.Fatal("run state write did not start")
	}

	blockDone := make(chan error, 1)
	go func() {
		blockDone <- hub.Publish(context.Background(), events.Envelope{
			Topic:       RuntimeBlockExecuteTopic("mcp/tool_call"),
			WorkspaceID: "ws-a",
			RunID:       "run-001",
			StepID:      "run-001-step-2",
			Payload: map[string]any{
				events.PayloadKeyRuntimeRequest: runtimemodel.StepExecutionRequest{
					WorkspaceID: "ws-a",
					RunID:       "run-001",
					StepID:      "run-001-step-2",
					Kind:        "mcp/tool_call",
				},
			},
		})
	}()

	select {
	case err := <-blockDone:
		if err != nil {
			if _, ok := events.ResponseEnvelope(err); !ok {
				t.Fatalf("runtime block command err = %v", err)
			}
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runtime block command blocked behind active run state lane")
	}

	close(releaseWrite)
	select {
	case err := <-writeDone:
		if err != nil {
			if _, ok := events.ResponseEnvelope(err); !ok {
				t.Fatalf("write command err = %v", err)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("write command did not finish")
	}
}

type noopRuntimeAdapter struct{}

func (noopRuntimeAdapter) ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return runtimemodel.StepExecutionResult{Output: map[string]any{"ok": true}}, nil
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
