package runtimeevent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestExecutorRunAndTeardownAreIdempotent(t *testing.T) {
	hub := magicevent.NewHub(16)
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(2)
	defer background.Shutdown(context.Background())
	executor := NewExecutor("test-block", hub, background, noopRuntimeAdapter{}, "test/kind")
	if err := executor.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	firstObserver := executor.observer
	if err := executor.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if executor.observer != firstObserver || len(executor.topics) != 1 {
		t.Fatalf("second Run changed subscriptions: topics=%v", executor.topics)
	}
	executor.Teardown()
	executor.Teardown()
	if executor.observer != nil || len(executor.topics) != 0 {
		t.Fatalf("Teardown retained subscriptions: %+v", executor)
	}
	if err := executor.Run(); err != nil {
		t.Fatalf("Run after Teardown: %v", err)
	}
}

func TestExecutorReturnsAcceptedBeforeAdapterCompletes(t *testing.T) {
	hub, background, executor := newRuntimeTestExecutor(t, newBlockingRuntimeAdapter(), "llm/workspace_execution")
	adapter := executor.adapter.(*blockingRuntimeAdapter)
	done := make(chan struct {
		result ExecuteCommandResult
		err    error
	}, 1)
	go func() {
		result, err := sendRuntimeCommand(hub, runtimeCommand("llm/workspace_execution", "request-001"))
		done <- struct {
			result ExecuteCommandResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case <-adapter.started:
	case <-time.After(time.Second):
		t.Fatal("adapter did not start")
	}
	select {
	case value := <-done:
		if value.err != nil {
			t.Fatalf("send command: %v", value.err)
		}
		if value.result.Accepted.RequestID != "request-001" || value.result.Accepted.QueueKey == "" {
			t.Fatalf("accepted = %+v", value.result.Accepted)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runtime command waited for adapter completion")
	}
	adapter.release(runtimemodel.StepExecutionResult{Output: map[string]any{"ok": true}}, nil)
	executor.Teardown()
	background.Shutdown(context.Background())
}

func TestExecutorPublishesTypedProgressAndCompletedData(t *testing.T) {
	adapter := runtimeAdapterFunc(func(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		if err := request.Progress(runtimemodel.StepExecutionProgress{Event: "provider_stream", Stage: "streaming", Data: map[string]any{"chunk": 1}}); err != nil {
			return runtimemodel.StepExecutionResult{}, err
		}
		return runtimemodel.StepExecutionResult{Output: map[string]any{"status": "ok"}}, nil
	})
	hub, _, _ := newRuntimeTestExecutor(t, adapter, "llm/workspace_execution")
	progressCh := make(chan ProgressData, 8)
	completedCh := make(chan CompletedData, 1)
	observer := subscribeRuntimeNotifications(hub, "typed-result-test", map[string]func(magicevent.Event){
		TopicRuntimeBlockExecuteProgress: func(event magicevent.Event) {
			data, ok := event.Data().(ProgressData)
			if !ok {
				t.Errorf("progress type = %T", event.Data())
				return
			}
			progressCh <- data
		},
		TopicRuntimeBlockExecuteCompleted: func(event magicevent.Event) {
			data, ok := event.Data().(CompletedData)
			if !ok {
				t.Errorf("completed type = %T", event.Data())
				return
			}
			completedCh <- data
		},
	})
	defer unsubscribeRuntimeNotifications(observer, TopicRuntimeBlockExecuteProgress, TopicRuntimeBlockExecuteCompleted)
	if _, err := sendRuntimeCommand(hub, runtimeCommand("llm/workspace_execution", "request-002")); err != nil {
		t.Fatalf("send command: %v", err)
	}
	for {
		select {
		case data := <-progressCh:
			if data.Progress.Event == "provider_stream" {
				if data.Progress.RequestID != "request-002" {
					t.Fatalf("progress request ID = %q", data.Progress.RequestID)
				}
				goto completed
			}
		case <-time.After(time.Second):
			t.Fatal("provider progress not published")
		}
	}

completed:
	select {
	case data := <-completedCh:
		if data.Completed.RequestID != "request-002" || data.Result.Output["status"] != "ok" {
			t.Fatalf("completed = %+v result=%#v", data.Completed, data.Result)
		}
	case <-time.After(time.Second):
		t.Fatal("completed data not published")
	}
}

func TestExecutorPublishesTypedFailedData(t *testing.T) {
	adapter := runtimeAdapterFunc(func(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		return runtimemodel.StepExecutionResult{Output: map[string]any{"partial": true}}, errors.New("provider unavailable")
	})
	hub, _, _ := newRuntimeTestExecutor(t, adapter, "llm/workspace_execution")
	failedCh := make(chan FailedData, 1)
	observer := subscribeRuntimeNotifications(hub, "typed-failure-test", map[string]func(magicevent.Event){
		TopicRuntimeBlockExecuteFailed: func(event magicevent.Event) {
			data, ok := event.Data().(FailedData)
			if !ok {
				t.Errorf("failed type = %T", event.Data())
				return
			}
			failedCh <- data
		},
	})
	defer unsubscribeRuntimeNotifications(observer, TopicRuntimeBlockExecuteFailed)
	if _, err := sendRuntimeCommand(hub, runtimeCommand("llm/workspace_execution", "request-003")); err != nil {
		t.Fatalf("send command: %v", err)
	}
	select {
	case data := <-failedCh:
		if data.Error != "provider unavailable" || data.Failed.RequestID != "request-003" || data.Result.Output["partial"] != true {
			t.Fatalf("failed data = %+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("failed data not published")
	}
}

func TestExecutorDuplicateCommandReplaysTypedTerminalDataWithoutReexecution(t *testing.T) {
	var calls atomic.Int64
	adapter := runtimeAdapterFunc(func(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		calls.Add(1)
		return runtimemodel.StepExecutionResult{Output: map[string]any{"status": "ok"}}, nil
	})
	hub, _, _ := newRuntimeTestExecutor(t, adapter, "llm/workspace_execution")
	completedCh := make(chan CompletedData, 2)
	observer := subscribeRuntimeNotifications(hub, "terminal-replay-test", map[string]func(magicevent.Event){
		TopicRuntimeBlockExecuteCompleted: func(event magicevent.Event) {
			completedCh <- event.Data().(CompletedData)
		},
	})
	defer unsubscribeRuntimeNotifications(observer, TopicRuntimeBlockExecuteCompleted)
	command := runtimeCommand("llm/workspace_execution", "request-replay")
	if _, err := sendRuntimeCommand(hub, command); err != nil {
		t.Fatalf("first command: %v", err)
	}
	waitCompletedData(t, completedCh)
	if _, err := sendRuntimeCommand(hub, command); err != nil {
		t.Fatalf("duplicate command: %v", err)
	}
	waitCompletedData(t, completedCh)
	if calls.Load() != 1 {
		t.Fatalf("adapter calls = %d, want 1", calls.Load())
	}
}

func TestExecutorTeardownPublishesWorkerLostProgress(t *testing.T) {
	adapter := newBlockingRuntimeAdapter()
	hub, _, executor := newRuntimeTestExecutor(t, adapter, "llm/workspace_execution")
	progressCh := make(chan ProgressData, 8)
	observer := subscribeRuntimeNotifications(hub, "worker-lost-test", map[string]func(magicevent.Event){
		TopicRuntimeBlockExecuteProgress: func(event magicevent.Event) {
			progressCh <- event.Data().(ProgressData)
		},
	})
	defer unsubscribeRuntimeNotifications(observer, TopicRuntimeBlockExecuteProgress)
	if _, err := sendRuntimeCommand(hub, runtimeCommand("llm/workspace_execution", "request-lost")); err != nil {
		t.Fatalf("send command: %v", err)
	}
	select {
	case <-adapter.started:
	case <-time.After(time.Second):
		t.Fatal("adapter did not start")
	}
	executor.Teardown()
	for {
		select {
		case data := <-progressCh:
			if data.Progress.Event == "runtime_worker_lost" {
				return
			}
		case <-time.After(time.Second):
			t.Fatal("worker lost progress not published")
		}
	}
}

func TestExecutorRejectsCommandAfterTeardown(t *testing.T) {
	hub, _, executor := newRuntimeTestExecutor(t, noopRuntimeAdapter{}, "llm/workspace_execution")
	executor.Teardown()
	observer := magicevent.NewSimpleObserverWithMatchID("closed-executor-test", CommandDestination, hub)
	observer.Subscribe(RuntimeBlockExecuteTopic("llm/workspace_execution"), executor.handle)
	defer observer.Unsubscribe(RuntimeBlockExecuteTopic("llm/workspace_execution"))
	_, err := sendRuntimeCommand(hub, runtimeCommand("llm/workspace_execution", "request-after-teardown"))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("command err = %v, want invalid argument", err)
	}
}

func newRuntimeTestExecutor(t *testing.T, adapter Adapter, kind string) (magicevent.Hub, magictask.BackgroundRoutine, *Executor) {
	t.Helper()
	hub := magicevent.NewHub(32)
	background := magictask.NewBackgroundRoutine(4)
	executor := NewExecutor("test-block", hub, background, adapter, kind)
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	t.Cleanup(func() {
		executor.Teardown()
		background.Shutdown(context.Background())
		hub.Terminate(context.Background())
	})
	return hub, background, executor
}

func sendRuntimeCommand(hub magicevent.Hub, command ExecuteCommand) (ExecuteCommandResult, error) {
	event := magicevent.NewEventWithContext(RuntimeBlockExecuteTopic(command.Request.Kind), "runtimeevent/test", CommandDestination, magicevent.NewHeader(), context.Background(), command)
	result := hub.Send(event)
	if result == nil {
		return ExecuteCommandResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return ExecuteCommandResult{}, err.Cause
		}
		return ExecuteCommandResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return ExecuteCommandResult{}, err
	}
	response, ok := data.(ExecuteCommandResult)
	if !ok {
		return ExecuteCommandResult{}, common.ErrInvalidArgument
	}
	return response, nil
}

func runtimeCommand(kind, requestID string) ExecuteCommand {
	return ExecuteCommand{
		RequestID: requestID,
		Request: runtimemodel.StepExecutionRequest{
			WorkspaceID: "ws-a",
			RunID:       "run-001",
			StepID:      "run-001-step-1",
			Kind:        kind,
		},
	}
}

func subscribeRuntimeNotifications(hub magicevent.Hub, id string, handlers map[string]func(magicevent.Event)) magicevent.SimpleObserver {
	observer := magicevent.NewSimpleObserverWithMatchID(id, NotificationDestination, hub)
	for topic, handler := range handlers {
		handler := handler
		observer.Subscribe(topic, func(event magicevent.Event, _ magicevent.Result) { handler(event) })
	}
	return observer
}

func unsubscribeRuntimeNotifications(observer magicevent.SimpleObserver, topics ...string) {
	for _, topic := range topics {
		observer.Unsubscribe(topic)
	}
}

func waitCompletedData(t *testing.T, ch <-chan CompletedData) CompletedData {
	t.Helper()
	select {
	case data := <-ch:
		return data
	case <-time.After(time.Second):
		t.Fatal("completed data not published")
		return CompletedData{}
	}
}

type noopRuntimeAdapter struct{}

func (noopRuntimeAdapter) ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return runtimemodel.StepExecutionResult{Output: map[string]any{"ok": true}}, nil
}

type runtimeAdapterFunc func(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error)

func (f runtimeAdapterFunc) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return f(ctx, request)
}

type blockingRuntimeAdapter struct {
	started   chan struct{}
	releaseCh chan blockingRuntimeResult
	calls     atomic.Int64
}

type blockingRuntimeResult struct {
	result runtimemodel.StepExecutionResult
	err    error
}

func newBlockingRuntimeAdapter() *blockingRuntimeAdapter {
	return &blockingRuntimeAdapter{started: make(chan struct{}), releaseCh: make(chan blockingRuntimeResult, 1)}
}

func (a *blockingRuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, _ runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	a.calls.Add(1)
	select {
	case <-a.started:
	default:
		close(a.started)
	}
	select {
	case value := <-a.releaseCh:
		return value.result, value.err
	case <-ctx.Done():
		return runtimemodel.StepExecutionResult{}, ctx.Err()
	}
}

func (a *blockingRuntimeAdapter) release(result runtimemodel.StepExecutionResult, err error) {
	a.releaseCh <- blockingRuntimeResult{result: result, err: err}
}
