package runtimeevent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	magictask "github.com/muidea/magicCommon/task"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestExecutorRunIsIdempotent(t *testing.T) {
	hub := &countingRuntimeHub{}
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())
	executor := NewExecutor("test-block", hub, background, noopRuntimeAdapter{}, "test/kind")
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
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	executor := NewExecutor("test-block", hub, background, noopRuntimeAdapter{}, "mcp/tool_call")
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

func TestExecutorRuntimeBlockCommandReturnsAcceptedBeforeAdapterCompletes(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	adapter := newBlockingRuntimeAdapter()
	executor := NewExecutor("test-block", hub, background, adapter, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	done := make(chan error, 1)
	go func() {
		done <- hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-001"))
	}()

	select {
	case <-adapter.started:
	case <-time.After(time.Second):
		t.Fatal("adapter did not start")
	}

	select {
	case err := <-done:
		response, ok := events.ResponseEnvelope(err)
		if !ok {
			t.Fatalf("runtime command err = %v, want accepted response", err)
		}
		accepted, ok := response.Payload[events.PayloadKeyRuntimeAccepted].(runtimemodel.RuntimeStepExecutionAccepted)
		if !ok {
			t.Fatalf("accepted payload missing: %#v", response.Payload)
		}
		if accepted.RequestID != "request-001" {
			t.Fatalf("accepted request id = %q, want request-001", accepted.RequestID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runtime command waited for adapter completion")
	}

	adapter.release(runtimemodel.StepExecutionResult{Output: map[string]any{"ok": true}}, nil)
}

func TestExecutorRunsDifferentWorkspaceRequestsConcurrently(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	var active atomic.Int64
	var calls atomic.Int64
	bothActive := make(chan struct{})
	release := make(chan struct{})
	adapter := runtimeAdapterFunc(func(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		calls.Add(1)
		if active.Add(1) == 2 {
			select {
			case <-bothActive:
			default:
				close(bothActive)
			}
		}
		defer active.Add(-1)
		select {
		case <-release:
		case <-ctx.Done():
			return runtimemodel.StepExecutionResult{}, ctx.Err()
		}
		return runtimemodel.StepExecutionResult{Output: map[string]any{"workspaceId": string(request.WorkspaceID)}}, nil
	})
	executor := NewExecutor("test-block", hub, background, adapter, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	for _, envelope := range []events.Envelope{
		runtimeCommandEnvelopeFor("llm/workspace_execution", "request-ws-a", "ws-a", "run-a", "run-a-step-1"),
		runtimeCommandEnvelopeFor("llm/workspace_execution", "request-ws-b", "ws-b", "run-b", "run-b-step-1"),
	} {
		if err := hub.Publish(context.Background(), envelope); err != nil {
			if _, ok := events.ResponseEnvelope(err); !ok {
				t.Fatalf("runtime command: %v", err)
			}
		}
	}

	select {
	case <-bothActive:
	case <-time.After(time.Second):
		t.Fatalf("runtime workers did not run concurrently; calls=%d active=%d", calls.Load(), active.Load())
	}
	close(release)
}

func TestExecutorPublishesCompletedAndProgressEvents(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	adapter := runtimeAdapterFunc(func(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		if request.Progress == nil {
			t.Fatal("progress callback missing")
		}
		if err := request.Progress(runtimemodel.StepExecutionProgress{
			Event:   "provider_stream",
			Stage:   "running",
			Summary: "streaming",
			Data:    map[string]any{"tokens": 3},
		}); err != nil {
			t.Fatalf("progress callback: %v", err)
		}
		return runtimemodel.StepExecutionResult{Output: map[string]any{"status": "completed"}}, nil
	})
	executor := NewExecutor("test-block", hub, background, adapter, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	progressCh := make(chan runtimemodel.RuntimeStepExecutionProgressed, 4)
	completedCh := make(chan runtimemodel.RuntimeStepExecutionCompleted, 1)
	unsubProgress := hub.Subscribe(TopicRuntimeBlockExecuteProgress, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeProgress].(runtimemodel.RuntimeStepExecutionProgressed); ok {
			progressCh <- payload
		}
		return nil
	})
	defer unsubProgress()
	unsubCompleted := hub.Subscribe(TopicRuntimeBlockExecuteCompleted, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeCompleted].(runtimemodel.RuntimeStepExecutionCompleted); ok {
			completedCh <- payload
		}
		return nil
	})
	defer unsubCompleted()

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-002")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("runtime command: %v", err)
		}
	}

	progress := waitRuntimeProgressEvent(t, progressCh, "provider_stream")
	if progress.RequestID != "request-002" || progress.Stage != "running" {
		t.Fatalf("unexpected progress payload: %#v", progress)
	}

	select {
	case completed := <-completedCh:
		if completed.RequestID != "request-002" {
			t.Fatalf("completed request id = %q, want request-002", completed.RequestID)
		}
		if completed.Output["status"] != "completed" {
			t.Fatalf("completed output = %#v", completed.Output)
		}
	case <-time.After(time.Second):
		t.Fatal("completed event not published")
	}
}

func TestExecutorProgressDoesNotCallOriginalRequestCallback(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	var originalProgressCalls atomic.Int64
	adapter := runtimeAdapterFunc(func(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		if request.Progress == nil {
			t.Fatal("progress publisher missing")
		}
		if err := request.Progress(runtimemodel.StepExecutionProgress{
			Event:   "provider_stream",
			Stage:   "running",
			Summary: "streaming",
			Data:    map[string]any{"tokens": 3},
		}); err != nil {
			t.Fatalf("progress publisher: %v", err)
		}
		return runtimemodel.StepExecutionResult{Output: map[string]any{"status": "completed"}}, nil
	})
	executor := NewExecutor("test-block", hub, background, adapter, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	progressCh := make(chan runtimemodel.RuntimeStepExecutionProgressed, 1)
	unsubProgress := hub.Subscribe(TopicRuntimeBlockExecuteProgress, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeProgress].(runtimemodel.RuntimeStepExecutionProgressed); ok && payload.Event == "provider_stream" {
			progressCh <- payload
		}
		return nil
	})
	defer unsubProgress()

	envelope := runtimeCommandEnvelope("llm/workspace_execution", "request-progress-event-only")
	request := envelope.Payload[events.PayloadKeyRuntimeRequest].(runtimemodel.StepExecutionRequest)
	request.Progress = func(runtimemodel.StepExecutionProgress) error {
		originalProgressCalls.Add(1)
		return nil
	}
	envelope.Payload[events.PayloadKeyRuntimeRequest] = request
	if err := hub.Publish(context.Background(), envelope); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("runtime command: %v", err)
		}
	}

	_ = waitRuntimeProgressEvent(t, progressCh, "provider_stream")
	if got := originalProgressCalls.Load(); got != 0 {
		t.Fatalf("original request progress calls = %d, want 0; progress must flow through runtime events", got)
	}
}

func TestExecutorPublishesWorkerLifecycleProgressEvents(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	executor := NewExecutor("test-block", hub, background, noopRuntimeAdapter{}, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	progressCh := make(chan runtimemodel.RuntimeStepExecutionProgressed, 4)
	unsubProgress := hub.Subscribe(TopicRuntimeBlockExecuteProgress, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeProgress].(runtimemodel.RuntimeStepExecutionProgressed); ok {
			progressCh <- payload
		}
		return nil
	})
	defer unsubProgress()

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-worker-lifecycle")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("runtime command: %v", err)
		}
	}

	started := waitRuntimeProgressEvent(t, progressCh, "runtime_worker_started")
	if started.Stage != "worker_started" || started.Data["workerId"] != "test-block|request-worker-lifecycle" {
		t.Fatalf("started progress = %#v, want worker lifecycle identity", started)
	}
	stopped := waitRuntimeProgressEvent(t, progressCh, "runtime_worker_stopped")
	if stopped.Stage != "worker_stopped" || stopped.Data["workerId"] != "test-block|request-worker-lifecycle" {
		t.Fatalf("stopped progress = %#v, want worker lifecycle identity", stopped)
	}
}

func TestExecutorPublishesWorkerLostOnTeardownForActiveRequest(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	adapter := newBlockingRuntimeAdapter()
	executor := NewExecutor("test-block", hub, background, adapter, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}

	progressCh := make(chan runtimemodel.RuntimeStepExecutionProgressed, 4)
	unsubProgress := hub.Subscribe(TopicRuntimeBlockExecuteProgress, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeProgress].(runtimemodel.RuntimeStepExecutionProgressed); ok {
			progressCh <- payload
		}
		return nil
	})
	defer unsubProgress()

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-worker-lost")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("runtime command: %v", err)
		}
	}
	select {
	case <-adapter.started:
	case <-time.After(time.Second):
		t.Fatal("adapter did not start")
	}
	executor.Teardown()

	lost := waitRuntimeProgressEvent(t, progressCh, "runtime_worker_lost")
	if lost.Stage != "worker_lost" || lost.Data["workerId"] != "test-block|request-worker-lost" {
		t.Fatalf("lost progress = %#v, want worker lost lifecycle identity", lost)
	}
	adapter.release(runtimemodel.StepExecutionResult{Output: map[string]any{"ok": true}}, nil)
}

func TestExecutorTeardownCancelsWorkerContext(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	adapterStarted := make(chan struct{})
	adapterCanceled := make(chan error, 1)
	adapter := runtimeAdapterFunc(func(ctx context.Context, _ runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		close(adapterStarted)
		<-ctx.Done()
		adapterCanceled <- ctx.Err()
		return runtimemodel.StepExecutionResult{Output: map[string]any{"cancelled": true}}, ctx.Err()
	})
	executor := NewExecutor("test-block", hub, background, adapter, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}

	progressCh := make(chan runtimemodel.RuntimeStepExecutionProgressed, 4)
	terminalCh := make(chan string, 2)
	unsubProgress := hub.Subscribe(TopicRuntimeBlockExecuteProgress, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeProgress].(runtimemodel.RuntimeStepExecutionProgressed); ok {
			progressCh <- payload
		}
		return nil
	})
	defer unsubProgress()
	unsubCompleted := hub.Subscribe(TopicRuntimeBlockExecuteCompleted, func(_ context.Context, _ events.Envelope) error {
		terminalCh <- TopicRuntimeBlockExecuteCompleted
		return nil
	})
	defer unsubCompleted()
	unsubFailed := hub.Subscribe(TopicRuntimeBlockExecuteFailed, func(_ context.Context, _ events.Envelope) error {
		terminalCh <- TopicRuntimeBlockExecuteFailed
		return nil
	})
	defer unsubFailed()

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-worker-cancel")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("runtime command: %v", err)
		}
	}
	select {
	case <-adapterStarted:
	case <-time.After(time.Second):
		t.Fatal("adapter did not start")
	}

	executor.Teardown()

	select {
	case err := <-adapterCanceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("adapter ctx err = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("adapter did not observe worker context cancellation")
	}
	lost := waitRuntimeProgressEvent(t, progressCh, "runtime_worker_lost")
	if lost.Stage != "worker_lost" || lost.Data["workerId"] != "test-block|request-worker-cancel" {
		t.Fatalf("lost progress = %#v, want worker lost lifecycle identity", lost)
	}
	select {
	case topic := <-terminalCh:
		t.Fatalf("terminal event %s published after teardown cancellation", topic)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestExecutorPublishesWorkerHeartbeatWhileRequestIsActive(t *testing.T) {
	originalInterval := runtimeWorkerHeartbeatInterval
	runtimeWorkerHeartbeatInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		runtimeWorkerHeartbeatInterval = originalInterval
	})

	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	adapter := newBlockingRuntimeAdapter()
	executor := NewExecutor("test-block", hub, background, adapter, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	progressCh := make(chan runtimemodel.RuntimeStepExecutionProgressed, 8)
	unsubProgress := hub.Subscribe(TopicRuntimeBlockExecuteProgress, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeProgress].(runtimemodel.RuntimeStepExecutionProgressed); ok {
			progressCh <- payload
		}
		return nil
	})
	defer unsubProgress()

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-worker-heartbeat")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("runtime command: %v", err)
		}
	}
	select {
	case <-adapter.started:
	case <-time.After(time.Second):
		t.Fatal("adapter did not start")
	}

	heartbeat := waitRuntimeProgressEvent(t, progressCh, "runtime_worker_heartbeat")
	if heartbeat.Stage != "worker_heartbeat" || heartbeat.Data["workerId"] != "test-block|request-worker-heartbeat" {
		t.Fatalf("heartbeat progress = %#v, want worker heartbeat identity", heartbeat)
	}
	adapter.release(runtimemodel.StepExecutionResult{Output: map[string]any{"ok": true}}, nil)
}

func TestExecutorPublishesFailedEvent(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	adapterErr := errors.New("provider unavailable")
	executor := NewExecutor("test-block", hub, background, runtimeAdapterFunc(func(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		return runtimemodel.StepExecutionResult{Output: map[string]any{"partial": true}}, adapterErr
	}), "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	failedCh := make(chan runtimemodel.RuntimeStepExecutionFailed, 1)
	unsubFailed := hub.Subscribe(TopicRuntimeBlockExecuteFailed, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeFailed].(runtimemodel.RuntimeStepExecutionFailed); ok {
			failedCh <- payload
		}
		return nil
	})
	defer unsubFailed()

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-003")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("runtime command: %v", err)
		}
	}

	select {
	case failed := <-failedCh:
		if failed.RequestID != "request-003" {
			t.Fatalf("failed request id = %q, want request-003", failed.RequestID)
		}
		if failed.Error != adapterErr.Error() {
			t.Fatalf("failed error = %q, want %q", failed.Error, adapterErr.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("failed event not published")
	}
}

func TestExecutorTerminalPublishFailureDoesNotRecordCompletedTerminal(t *testing.T) {
	publishErr := errors.New("completed publish failed")
	hub := &failingRuntimePublishHub{failTopic: TopicRuntimeBlockExecuteCompleted, err: publishErr}
	executor := NewExecutor("test-block", hub, nil, nil, "llm/workspace_execution")
	request := runtimemodel.StepExecutionRequest{
		WorkspaceID: "ws-terminal-publish-fail",
		RunID:       "run-terminal-publish-fail",
		StepID:      "run-terminal-publish-fail/step-1",
		Kind:        "llm/workspace_execution",
	}
	requestID := "request-terminal-publish-fail"
	queueKey := runtimeQueueKey(request, requestID)
	if _, _, err := executor.accept(queueKey, request, requestID); err != nil {
		t.Fatalf("accept request: %v", err)
	}

	err := executor.publishCompleted(context.Background(), request, requestID, runtimemodel.StepExecutionResult{Output: map[string]any{"status": "completed"}})
	if !errors.Is(err, publishErr) {
		t.Fatalf("publishCompleted err = %v, want %v", err, publishErr)
	}

	executor.mu.Lock()
	state := executor.requests[queueKey]
	executor.mu.Unlock()
	if state.Terminal || state.TerminalEvent.Topic != "" {
		t.Fatalf("terminal state = %+v, want no terminal replay recorded after publish failure", state)
	}
}

func TestExecutorWorkerLostPublishFailureIsReturned(t *testing.T) {
	publishErr := errors.New("progress publish failed")
	hub := &failingRuntimePublishHub{failTopic: TopicRuntimeBlockExecuteProgress, err: publishErr}
	executor := NewExecutor("test-block", hub, nil, nil, "llm/workspace_execution")
	request := runtimemodel.StepExecutionRequest{
		WorkspaceID: "ws-worker-lost-publish-fail",
		RunID:       "run-worker-lost-publish-fail",
		StepID:      "run-worker-lost-publish-fail/step-1",
		Kind:        "llm/workspace_execution",
	}

	err := executor.publishWorkerLifecycle(context.Background(), request, "request-worker-lost-publish-fail", "runtime_worker_lost", "worker_lost", "runtime worker lost", map[string]any{
		"queueKey": "queue-worker-lost-publish-fail",
		"workerId": "test-block|request-worker-lost-publish-fail",
	})
	if !errors.Is(err, publishErr) {
		t.Fatalf("publishWorkerLifecycle err = %v, want %v", err, publishErr)
	}
}

func TestExecutorAcceptedPublishFailureDoesNotStartWorker(t *testing.T) {
	publishErr := errors.New("accepted publish failed")
	hub := &failingRuntimePublishHub{failTopic: TopicRuntimeBlockExecuteAccepted, err: publishErr}
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	var calls atomic.Int64
	started := make(chan struct{})
	executor := NewExecutor("test-block", hub, background, runtimeAdapterFunc(func(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		calls.Add(1)
		close(started)
		return runtimemodel.StepExecutionResult{Output: map[string]any{"unexpected": true}}, nil
	}), "llm/workspace_execution")

	err := executor.handle(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-accepted-publish-fail"))
	if !errors.Is(err, publishErr) {
		t.Fatalf("handle err = %v, want %v", err, publishErr)
	}

	select {
	case <-started:
		t.Fatal("runtime worker started after accepted publish failure")
	case <-time.After(100 * time.Millisecond):
	}
	if calls.Load() != 0 {
		t.Fatalf("adapter calls = %d, want 0", calls.Load())
	}

	queueKey := runtimeQueueKey(runtimeCommandEnvelope("llm/workspace_execution", "request-accepted-publish-fail").Payload[events.PayloadKeyRuntimeRequest].(runtimemodel.StepExecutionRequest), "request-accepted-publish-fail")
	executor.mu.Lock()
	_, exists := executor.requests[queueKey]
	executor.mu.Unlock()
	if exists {
		t.Fatalf("queue key %q retained after accepted publish failure", queueKey)
	}
}

func TestExecutorTerminalReplayPublishFailureIsReturned(t *testing.T) {
	publishErr := errors.New("terminal replay publish failed")
	hub := &failingRuntimePublishHub{failTopic: TopicRuntimeBlockExecuteCompleted, err: publishErr}
	executor := NewExecutor("test-block", hub, nil, nil, "llm/workspace_execution")
	envelope := runtimeCommandEnvelope("llm/workspace_execution", "request-terminal-replay-fail")
	request := envelope.Payload[events.PayloadKeyRuntimeRequest].(runtimemodel.StepExecutionRequest)
	requestID := "request-terminal-replay-fail"
	queueKey := runtimeQueueKey(request, requestID)
	if _, _, err := executor.accept(queueKey, request, requestID); err != nil {
		t.Fatalf("accept request: %v", err)
	}
	executor.recordTerminal(request, requestID, events.Envelope{
		Topic:       TopicRuntimeBlockExecuteCompleted,
		WorkspaceID: string(request.WorkspaceID),
		RunID:       string(request.RunID),
		StepID:      string(request.StepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeCompleted: runtimemodel.RuntimeStepExecutionCompleted{
				WorkspaceID: request.WorkspaceID,
				RunID:       request.RunID,
				StepID:      request.StepID,
				RequestID:   requestID,
			},
		},
	})

	err := executor.handle(context.Background(), envelope)
	if !errors.Is(err, publishErr) {
		t.Fatalf("terminal replay err = %v, want %v", err, publishErr)
	}
}

func TestExecutorDuplicateRequestIDExecutesAdapterOnce(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	adapter := newBlockingRuntimeAdapter()
	executor := NewExecutor("test-block", hub, background, adapter, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-duplicate")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("first runtime command: %v", err)
		}
	}
	select {
	case <-adapter.started:
	case <-time.After(time.Second):
		t.Fatal("adapter did not start")
	}
	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-duplicate")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("duplicate runtime command: %v", err)
		}
	}

	adapter.release(runtimemodel.StepExecutionResult{Output: map[string]any{"ok": true}}, nil)
	deadline := time.After(100 * time.Millisecond)
	for {
		if adapter.calls.Load() == 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("adapter calls = %d, want 1", adapter.calls.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestExecutorDuplicateCompletedRequestReplaysTerminalEvent(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	var calls atomic.Int64
	executor := NewExecutor("test-block", hub, background, runtimeAdapterFunc(func(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		calls.Add(1)
		return runtimemodel.StepExecutionResult{Output: map[string]any{"status": "completed"}}, nil
	}), "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	completedCh := make(chan runtimemodel.RuntimeStepExecutionCompleted, 2)
	unsubCompleted := hub.Subscribe(TopicRuntimeBlockExecuteCompleted, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeCompleted].(runtimemodel.RuntimeStepExecutionCompleted); ok {
			completedCh <- payload
		}
		return nil
	})
	defer unsubCompleted()

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-replay-completed")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("first runtime command: %v", err)
		}
	}
	first := waitCompletedRuntimeEvent(t, completedCh)
	if first.Output["status"] != "completed" {
		t.Fatalf("first completed output = %#v", first.Output)
	}

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-replay-completed")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("duplicate runtime command: %v", err)
		}
	}
	second := waitCompletedRuntimeEvent(t, completedCh)
	if second.RequestID != first.RequestID || second.Output["status"] != "completed" {
		t.Fatalf("replayed completed = %#v, want same request completed output", second)
	}
	if calls.Load() != 1 {
		t.Fatalf("adapter calls = %d, want 1", calls.Load())
	}
}

func TestExecutorDuplicateFailedRequestReplaysTerminalEvent(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	var calls atomic.Int64
	adapterErr := errors.New("provider unavailable")
	executor := NewExecutor("test-block", hub, background, runtimeAdapterFunc(func(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
		calls.Add(1)
		return runtimemodel.StepExecutionResult{Output: map[string]any{"partial": true}}, adapterErr
	}), "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	defer executor.Teardown()

	failedCh := make(chan runtimemodel.RuntimeStepExecutionFailed, 2)
	unsubFailed := hub.Subscribe(TopicRuntimeBlockExecuteFailed, func(_ context.Context, envelope events.Envelope) error {
		if payload, ok := envelope.Payload[events.PayloadKeyRuntimeFailed].(runtimemodel.RuntimeStepExecutionFailed); ok {
			failedCh <- payload
		}
		return nil
	})
	defer unsubFailed()

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-replay-failed")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("first runtime command: %v", err)
		}
	}
	first := waitFailedRuntimeEvent(t, failedCh)
	if first.Error != adapterErr.Error() {
		t.Fatalf("first failed error = %q, want %q", first.Error, adapterErr.Error())
	}

	if err := hub.Publish(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-replay-failed")); err != nil {
		if _, ok := events.ResponseEnvelope(err); !ok {
			t.Fatalf("duplicate runtime command: %v", err)
		}
	}
	second := waitFailedRuntimeEvent(t, failedCh)
	if second.RequestID != first.RequestID || second.Error != adapterErr.Error() {
		t.Fatalf("replayed failed = %#v, want same request failure", second)
	}
	if calls.Load() != 1 {
		t.Fatalf("adapter calls = %d, want 1", calls.Load())
	}
}

func TestExecutorRejectsCommandAfterTeardown(t *testing.T) {
	hub := events.NewMagicHub()
	defer hub.Terminate(context.Background())
	background := magictask.NewBackgroundRoutine(4)
	defer background.Shutdown(context.Background())

	executor := NewExecutor("test-block", hub, background, noopRuntimeAdapter{}, "llm/workspace_execution")
	if err := executor.Run(); err != nil {
		t.Fatalf("executor Run: %v", err)
	}
	executor.Teardown()

	err := executor.handle(context.Background(), runtimeCommandEnvelope("llm/workspace_execution", "request-after-teardown"))
	if err == nil {
		t.Fatal("handle after teardown err = nil, want error")
	}
}

func waitCompletedRuntimeEvent(t *testing.T, ch <-chan runtimemodel.RuntimeStepExecutionCompleted) runtimemodel.RuntimeStepExecutionCompleted {
	t.Helper()
	select {
	case item := <-ch:
		return item
	case <-time.After(time.Second):
		t.Fatal("completed event not published")
		return runtimemodel.RuntimeStepExecutionCompleted{}
	}
}

func waitFailedRuntimeEvent(t *testing.T, ch <-chan runtimemodel.RuntimeStepExecutionFailed) runtimemodel.RuntimeStepExecutionFailed {
	t.Helper()
	select {
	case item := <-ch:
		return item
	case <-time.After(time.Second):
		t.Fatal("failed event not published")
		return runtimemodel.RuntimeStepExecutionFailed{}
	}
}

func waitRuntimeProgressEvent(t *testing.T, ch <-chan runtimemodel.RuntimeStepExecutionProgressed, event string) runtimemodel.RuntimeStepExecutionProgressed {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case item := <-ch:
			if item.Event == event {
				return item
			}
		case <-deadline:
			t.Fatalf("runtime progress event %q not published", event)
			return runtimemodel.RuntimeStepExecutionProgressed{}
		}
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
	return &blockingRuntimeAdapter{
		started:   make(chan struct{}),
		releaseCh: make(chan blockingRuntimeResult, 1),
	}
}

func (a *blockingRuntimeAdapter) ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	a.calls.Add(1)
	select {
	case <-a.started:
	default:
		close(a.started)
	}
	value := <-a.releaseCh
	return value.result, value.err
}

func (a *blockingRuntimeAdapter) release(result runtimemodel.StepExecutionResult, err error) {
	a.releaseCh <- blockingRuntimeResult{result: result, err: err}
}

func runtimeCommandEnvelope(kind, requestID string) events.Envelope {
	return runtimeCommandEnvelopeFor(kind, requestID, "ws-a", "run-001", "run-001-step-1")
}

func runtimeCommandEnvelopeFor(kind, requestID, workspaceID, runID, stepID string) events.Envelope {
	return events.Envelope{
		Topic:       RuntimeBlockExecuteTopic(kind),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Payload: map[string]any{
			events.PayloadKeyRuntimeRequest: runtimemodel.StepExecutionRequest{
				WorkspaceID: common.WorkspaceID(workspaceID),
				RunID:       common.RunID(runID),
				StepID:      common.StepID(stepID),
				Kind:        kind,
			},
			events.PayloadKeyRuntimeRequestID: requestID,
		},
	}
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

type failingRuntimePublishHub struct {
	failTopic string
	err       error
	published []events.Envelope
}

func (h *failingRuntimePublishHub) Publish(_ context.Context, envelope events.Envelope) error {
	if envelope.Topic == h.failTopic {
		return h.err
	}
	h.published = append(h.published, envelope)
	return nil
}

func (h *failingRuntimePublishHub) Subscribe(string, events.Handler) func() {
	return func() {}
}
