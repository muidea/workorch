package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestEventRuntimeAdapterExecuteRuntimeStepReturnsCompletedResult(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	executor := runtimeevent.NewExecutor("runtime-test", hub, background, runtimeAdapterFunc(func(_ context.Context, _ runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		return runtimemodel.StepExecutionResult{Output: map[string]any{"status": "ok"}}, nil
	}), "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	adapter := NewEventRuntimeAdapter(hub)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimeStepRequest("llm/workspace_execution"))
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["status"] != "ok" {
		t.Fatalf("result output = %#v, want status ok", result.Output)
	}
}

func TestEventRuntimeAdapterRequestRuntimeStepExecutionReturnsAcceptedBeforeResult(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	started := make(chan struct{})
	release := make(chan struct{})
	executor := runtimeevent.NewExecutor("runtime-test", hub, background, runtimeAdapterFunc(func(_ context.Context, _ runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		close(started)
		<-release
		return runtimemodel.StepExecutionResult{Output: map[string]any{"status": "ok"}}, nil
	}), "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	adapter := NewEventRuntimeAdapter(hub).(interface {
		RequestRuntimeStepExecution(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.RuntimeStepExecutionAccepted, error)
	})
	done := make(chan struct {
		accepted runtimemodel.RuntimeStepExecutionAccepted
		err      error
	}, 1)
	go func() {
		accepted, err := adapter.RequestRuntimeStepExecution(context.Background(), runtimeStepRequest("llm/workspace_execution"))
		done <- struct {
			accepted runtimemodel.RuntimeStepExecutionAccepted
			err      error
		}{accepted: accepted, err: err}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runtime adapter did not start")
	}
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("RequestRuntimeStepExecution: %v", result.err)
		}
		if result.accepted.RequestID == "" || result.accepted.QueueKey == "" {
			t.Fatalf("accepted = %+v, want request and queue key", result.accepted)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("RequestRuntimeStepExecution waited for runtime result")
	}
	close(release)
}

func TestEventRuntimeAdapterExecuteRuntimeStepReturnsFailedEventError(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	executor := runtimeevent.NewExecutor("runtime-test", hub, background, runtimeAdapterFunc(func(_ context.Context, _ runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		return runtimemodel.StepExecutionResult{Output: map[string]any{"partial": true}}, errors.New("provider unavailable")
	}), "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	adapter := NewEventRuntimeAdapter(hub)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimeStepRequest("llm/workspace_execution"))
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want provider unavailable")
	}
	if err.Error() != "provider unavailable" {
		t.Fatalf("ExecuteRuntimeStep err = %q, want provider unavailable", err.Error())
	}
	if result.Output["partial"] != true {
		t.Fatalf("result output = %#v, want partial true", result.Output)
	}
}

func TestEventRuntimeAdapterExecuteRuntimeStepReturnsContextErrorWhenNoResultArrives(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	blocked := make(chan struct{})
	executor := runtimeevent.NewExecutor("runtime-test", hub, background, runtimeAdapterFunc(func(_ context.Context, _ runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		<-blocked
		return runtimemodel.StepExecutionResult{}, nil
	}), "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	adapter := NewEventRuntimeAdapter(hub)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := adapter.ExecuteRuntimeStep(ctx, runtimeStepRequest("llm/workspace_execution"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecuteRuntimeStep err = %v, want deadline exceeded", err)
	}
	close(blocked)
}

type runtimeAdapterFunc func(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error)

func (f runtimeAdapterFunc) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return f(ctx, request)
}

func runtimeStepRequest(kind string) runtimemodel.StepExecutionRequest {
	return runtimemodel.StepExecutionRequest{
		WorkspaceID: "ws-a",
		RunID:       "run-001",
		StepID:      "run-001-step-1",
		Kind:        kind,
	}
}
