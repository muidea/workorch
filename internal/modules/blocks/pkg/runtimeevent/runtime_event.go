package runtimeevent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Adapter interface {
	ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error)
}

var runtimeWorkerHeartbeatInterval = 30 * time.Second

type Executor struct {
	moduleID   string
	hub        events.Hub
	background task.BackgroundRoutine
	adapter    Adapter
	kinds      []string
	unsub      []func()

	mu       sync.Mutex
	closed   bool
	requests map[string]runtimeExecutionState
}

type runtimeExecutionState struct {
	Accepted      bool
	Terminal      bool
	Request       runtimemodel.StepExecutionRequest
	RequestID     string
	QueueKey      string
	WorkerID      string
	WorkerCancel  context.CancelFunc
	TerminalEvent events.Envelope
}

func NewExecutor(moduleID string, hub events.Hub, background task.BackgroundRoutine, adapter Adapter, kinds ...string) *Executor {
	return &Executor{
		moduleID:   moduleID,
		hub:        hub,
		background: background,
		adapter:    adapter,
		kinds:      append([]string(nil), kinds...),
		requests:   map[string]runtimeExecutionState{},
	}
}

func (e *Executor) Run() error {
	if e == nil || e.hub == nil || e.background == nil || e.adapter == nil || len(e.kinds) == 0 {
		return common.ErrInvalidArgument
	}
	if len(e.unsub) > 0 {
		return nil
	}
	e.mu.Lock()
	e.closed = false
	if e.requests == nil {
		e.requests = map[string]runtimeExecutionState{}
	}
	e.mu.Unlock()
	for _, kind := range e.kinds {
		if kind == "" {
			return common.ErrInvalidArgument
		}
		e.unsub = append(e.unsub, e.hub.Subscribe(RuntimeBlockExecuteTopic(kind), e.handle))
	}
	return nil
}

func (e *Executor) Teardown() {
	if e == nil {
		return
	}
	for idx := len(e.unsub) - 1; idx >= 0; idx-- {
		if e.unsub[idx] != nil {
			e.unsub[idx]()
		}
	}
	e.unsub = nil
	var lost []runtimeExecutionState
	var cancels []context.CancelFunc
	e.mu.Lock()
	e.closed = true
	for _, state := range e.requests {
		if state.Accepted && !state.Terminal {
			lost = append(lost, state)
			if state.WorkerCancel != nil {
				cancels = append(cancels, state.WorkerCancel)
			}
		}
	}
	e.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	for _, state := range lost {
		_ = e.publishWorkerLifecycle(context.Background(), state.Request, state.RequestID, "runtime_worker_lost", "worker_lost", "runtime worker lost before terminal result", map[string]any{
			"queueKey": state.QueueKey,
			"workerId": state.WorkerID,
			"lostAt":   nowRFC3339(),
		})
	}
}

func (e *Executor) handle(ctx context.Context, envelope events.Envelope) error {
	request, ok := envelope.Payload[events.PayloadKeyRuntimeRequest].(runtimemodel.StepExecutionRequest)
	if !ok {
		return common.ErrInvalidArgument
	}
	if request.Kind == "" {
		request.Kind = runtimeKindFromTopic(envelope.Topic)
	}
	if request.WorkspaceID == "" && envelope.WorkspaceID != "" {
		request.WorkspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	if request.RunID == "" && envelope.RunID != "" {
		request.RunID = common.RunID(envelope.RunID)
	}
	if request.StepID == "" && envelope.StepID != "" {
		request.StepID = common.StepID(envelope.StepID)
	}
	if request.Kind == "" || request.StepID == "" {
		return common.ErrInvalidArgument
	}
	requestID := runtimeRequestID(envelope, request)
	queueKey := runtimeQueueKey(request, requestID)
	accepted := runtimemodel.RuntimeStepExecutionAccepted{
		WorkspaceID: request.WorkspaceID,
		RunID:       request.RunID,
		StepID:      request.StepID,
		StepKind:    request.Kind,
		RequestID:   requestID,
		QueueKey:    queueKey,
		AcceptedAt:  nowRFC3339(),
	}
	schedule, replay, err := e.accept(queueKey, request, requestID)
	if err != nil {
		return err
	}
	if schedule {
		workerRequest := request
		workerRequest.Progress = e.progressPublisher(request, requestID)
		workerBaseCtx := context.Background()
		if ctx != nil {
			workerBaseCtx = context.WithoutCancel(ctx)
		}
		workerCtx, workerCancel := context.WithCancel(workerBaseCtx)
		e.markWorkerCancel(queueKey, workerCancel)
		if err := e.background.AsyncFunction(func() {
			e.execute(workerCtx, workerRequest, requestID)
		}); err != nil {
			workerCancel()
			e.forget(queueKey)
			return err
		}
	} else if replay.Topic != "" {
		_ = publishRuntimeNotification(e.hub, ctx, replay)
	}
	_ = e.publishAccepted(ctx, accepted)
	payload := map[string]any{
		events.PayloadKeyRuntimeAccepted:  accepted,
		events.PayloadKeyRuntimeRequestID: requestID,
		events.PayloadKeyRuntimeQueueKey:  queueKey,
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: e.moduleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload:      payload,
	})
}

func (e *Executor) accept(queueKey string, request runtimemodel.StepExecutionRequest, requestID string) (bool, events.Envelope, error) {
	if e == nil || queueKey == "" {
		return false, events.Envelope{}, common.ErrInvalidArgument
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return false, events.Envelope{}, fmt.Errorf("%w: runtime executor is closed", common.ErrInvalidArgument)
	}
	if e.requests == nil {
		e.requests = map[string]runtimeExecutionState{}
	}
	if state := e.requests[queueKey]; state.Accepted {
		if state.Terminal {
			return false, state.TerminalEvent, nil
		}
		return false, events.Envelope{}, nil
	}
	e.requests[queueKey] = runtimeExecutionState{
		Accepted:  true,
		Request:   request,
		RequestID: requestID,
		QueueKey:  queueKey,
		WorkerID:  runtimeWorkerID(e.moduleID, requestID),
	}
	return true, events.Envelope{}, nil
}

func (e *Executor) forget(queueKey string) {
	if e == nil || queueKey == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.requests, queueKey)
}

func (e *Executor) execute(ctx context.Context, request runtimemodel.StepExecutionRequest, requestID string) {
	queueKey := runtimeQueueKey(request, requestID)
	workerID := runtimeWorkerID(e.moduleID, requestID)
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	e.startWorkerHeartbeat(heartbeatCtx, request, requestID, queueKey, workerID)
	startedAt := nowRFC3339()
	_ = e.publishWorkerLifecycle(ctx, request, requestID, "runtime_worker_started", "worker_started", "runtime worker started", map[string]any{
		"queueKey":  queueKey,
		"workerId":  workerID,
		"startedAt": startedAt,
	})
	result, err := e.adapter.ExecuteRuntimeStep(ctx, request)
	if e.closedBeforeTerminal() && ctx.Err() != nil {
		_ = e.publishWorkerLifecycle(context.Background(), request, requestID, "runtime_worker_stopped", "worker_stopped", "runtime worker stopped after cancellation", map[string]any{
			"queueKey":  queueKey,
			"workerId":  workerID,
			"startedAt": startedAt,
			"stoppedAt": nowRFC3339(),
			"cancelled": true,
		})
		return
	}
	if err != nil {
		_ = e.publishFailed(ctx, request, requestID, result, err)
		_ = e.publishWorkerLifecycle(ctx, request, requestID, "runtime_worker_stopped", "worker_stopped", "runtime worker stopped", map[string]any{
			"queueKey":  queueKey,
			"workerId":  workerID,
			"startedAt": startedAt,
			"stoppedAt": nowRFC3339(),
		})
		return
	}
	_ = e.publishCompleted(ctx, request, requestID, result)
	_ = e.publishWorkerLifecycle(ctx, request, requestID, "runtime_worker_stopped", "worker_stopped", "runtime worker stopped", map[string]any{
		"queueKey":  queueKey,
		"workerId":  workerID,
		"startedAt": startedAt,
		"stoppedAt": nowRFC3339(),
	})
}

func (e *Executor) markWorkerCancel(queueKey string, cancel context.CancelFunc) {
	if e == nil || queueKey == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	state := e.requests[queueKey]
	state.WorkerCancel = cancel
	e.requests[queueKey] = state
}

func (e *Executor) closedBeforeTerminal() bool {
	if e == nil {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closed
}

func (e *Executor) startWorkerHeartbeat(ctx context.Context, request runtimemodel.StepExecutionRequest, requestID string, queueKey string, workerID string) {
	if e == nil || e.background == nil || runtimeWorkerHeartbeatInterval <= 0 {
		return
	}
	_ = e.background.Timer(ctx, runtimeWorkerHeartbeatTask{fn: func() {
		_ = e.publishWorkerLifecycle(context.Background(), request, requestID, "runtime_worker_heartbeat", "worker_heartbeat", "runtime worker heartbeat", map[string]any{
			"queueKey":     queueKey,
			"workerId":     workerID,
			"heartbeatAt":  nowRFC3339(),
			"heartbeatTTL": runtimeWorkerHeartbeatInterval.String(),
		})
	}}, runtimeWorkerHeartbeatInterval, 0)
}

type runtimeWorkerHeartbeatTask struct {
	fn func()
}

func (t runtimeWorkerHeartbeatTask) Run() {
	if t.fn != nil {
		t.fn()
	}
}

func (e *Executor) publishWorkerLifecycle(ctx context.Context, request runtimemodel.StepExecutionRequest, requestID string, event string, stage string, summary string, data map[string]any) error {
	if e == nil {
		return common.ErrInvalidArgument
	}
	progress := runtimemodel.RuntimeStepExecutionProgressed{
		WorkspaceID: request.WorkspaceID,
		RunID:       request.RunID,
		StepID:      request.StepID,
		StepKind:    request.Kind,
		RequestID:   requestID,
		Event:       event,
		Stage:       stage,
		Summary:     summary,
		Data:        cloneMap(data),
		OccurredAt:  nowRFC3339(),
	}
	return publishRuntimeNotification(e.hub, ctx, events.Envelope{
		Topic:        TopicRuntimeBlockExecuteProgress,
		SourceModule: e.moduleID,
		SubjectType:  "runtime_step",
		SubjectID:    string(request.StepID),
		WorkspaceID:  string(request.WorkspaceID),
		RunID:        string(request.RunID),
		StepID:       string(request.StepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeProgress:  progress,
			events.PayloadKeyRuntimeRequestID: requestID,
		},
	})
}

func (e *Executor) progressPublisher(request runtimemodel.StepExecutionRequest, requestID string) runtimemodel.StepExecutionProgressFunc {
	return func(progress runtimemodel.StepExecutionProgress) error {
		payload := runtimemodel.RuntimeStepExecutionProgressed{
			WorkspaceID: request.WorkspaceID,
			RunID:       request.RunID,
			StepID:      request.StepID,
			StepKind:    request.Kind,
			RequestID:   requestID,
			Event:       progress.Event,
			Stage:       progress.Stage,
			Summary:     progress.Summary,
			Data:        cloneMap(progress.Data),
			OccurredAt:  nowRFC3339(),
		}
		_ = publishRuntimeNotification(e.hub, context.Background(), events.Envelope{
			Topic:        TopicRuntimeBlockExecuteProgress,
			SourceModule: e.moduleID,
			SubjectType:  "runtime_step",
			SubjectID:    string(request.StepID),
			WorkspaceID:  string(request.WorkspaceID),
			RunID:        string(request.RunID),
			StepID:       string(request.StepID),
			Payload: map[string]any{
				events.PayloadKeyRuntimeProgress:  payload,
				events.PayloadKeyRuntimeRequestID: requestID,
			},
		})
		return nil
	}
}

func (e *Executor) publishAccepted(ctx context.Context, accepted runtimemodel.RuntimeStepExecutionAccepted) error {
	return publishRuntimeNotification(e.hub, ctx, events.Envelope{
		Topic:        TopicRuntimeBlockExecuteAccepted,
		SourceModule: e.moduleID,
		SubjectType:  "runtime_step",
		SubjectID:    string(accepted.StepID),
		WorkspaceID:  string(accepted.WorkspaceID),
		RunID:        string(accepted.RunID),
		StepID:       string(accepted.StepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeAccepted:  accepted,
			events.PayloadKeyRuntimeRequestID: accepted.RequestID,
			events.PayloadKeyRuntimeQueueKey:  accepted.QueueKey,
		},
	})
}

func (e *Executor) publishCompleted(ctx context.Context, request runtimemodel.StepExecutionRequest, requestID string, result runtimemodel.StepExecutionResult) error {
	completed := runtimemodel.RuntimeStepExecutionCompleted{
		WorkspaceID: request.WorkspaceID,
		RunID:       request.RunID,
		StepID:      request.StepID,
		StepKind:    request.Kind,
		RequestID:   requestID,
		Output:      cloneMap(result.Output),
		CompletedAt: nowRFC3339(),
	}
	envelope := events.Envelope{
		Topic:        TopicRuntimeBlockExecuteCompleted,
		SourceModule: e.moduleID,
		SubjectType:  "runtime_step",
		SubjectID:    string(request.StepID),
		WorkspaceID:  string(request.WorkspaceID),
		RunID:        string(request.RunID),
		StepID:       string(request.StepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeCompleted: completed,
			events.PayloadKeyRuntimeRequestID: requestID,
			events.PayloadKeyRuntimeResult:    result,
		},
	}
	e.recordTerminal(request, requestID, envelope)
	return publishRuntimeNotification(e.hub, ctx, envelope)
}

func (e *Executor) publishFailed(ctx context.Context, request runtimemodel.StepExecutionRequest, requestID string, result runtimemodel.StepExecutionResult, execErr error) error {
	failed := runtimemodel.RuntimeStepExecutionFailed{
		WorkspaceID:  request.WorkspaceID,
		RunID:        request.RunID,
		StepID:       request.StepID,
		StepKind:     request.Kind,
		RequestID:    requestID,
		Error:        execErr.Error(),
		CompletedAt:  nowRFC3339(),
		RawResultRef: "",
	}
	envelope := events.Envelope{
		Topic:        TopicRuntimeBlockExecuteFailed,
		SourceModule: e.moduleID,
		SubjectType:  "runtime_step",
		SubjectID:    string(request.StepID),
		WorkspaceID:  string(request.WorkspaceID),
		RunID:        string(request.RunID),
		StepID:       string(request.StepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeFailed:    failed,
			events.PayloadKeyRuntimeRequestID: requestID,
			events.PayloadKeyRuntimeResult:    result,
			events.PayloadKeyError:            execErr.Error(),
		},
	}
	e.recordTerminal(request, requestID, envelope)
	return publishRuntimeNotification(e.hub, ctx, envelope)
}

func (e *Executor) recordTerminal(request runtimemodel.StepExecutionRequest, requestID string, envelope events.Envelope) {
	if e == nil {
		return
	}
	queueKey := runtimeQueueKey(request, requestID)
	if queueKey == "" || envelope.Topic == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	state := e.requests[queueKey]
	state.Accepted = true
	state.Terminal = true
	state.Request = request
	state.RequestID = requestID
	state.QueueKey = queueKey
	state.WorkerID = runtimeWorkerID(e.moduleID, requestID)
	state.TerminalEvent = cloneEnvelope(envelope)
	e.requests[queueKey] = state
}

func publishRuntimeNotification(hub events.Hub, ctx context.Context, envelope events.Envelope) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	if asyncHub, ok := hub.(events.AsyncHub); ok {
		return asyncHub.PublishAsync(ctx, envelope)
	}
	err := hub.Publish(ctx, envelope)
	if _, ok := events.ResponseEnvelope(err); ok {
		return nil
	}
	return err
}

func runtimeRequestID(envelope events.Envelope, request runtimemodel.StepExecutionRequest) string {
	if value, _ := envelope.Payload[events.PayloadKeyRuntimeRequestID].(string); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	parts := []string{string(request.WorkspaceID), string(request.RunID), string(request.StepID), request.Kind}
	return strings.Join(parts, "|")
}

func runtimeQueueKey(request runtimemodel.StepExecutionRequest, requestID string) string {
	return strings.Join([]string{string(request.WorkspaceID), string(request.RunID), string(request.StepID), requestID}, "|")
}

func runtimeWorkerID(moduleID string, requestID string) string {
	moduleID = strings.TrimSpace(moduleID)
	requestID = strings.TrimSpace(requestID)
	if moduleID == "" {
		return requestID
	}
	if requestID == "" {
		return moduleID
	}
	return moduleID + "|" + requestID
}

func runtimeKindFromTopic(topic string) string {
	if strings.HasPrefix(topic, TopicRuntimeBlockCommandExecutePrefix) {
		return strings.TrimSpace(strings.TrimPrefix(topic, TopicRuntimeBlockCommandExecutePrefix))
	}
	return ""
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneEnvelope(input events.Envelope) events.Envelope {
	ret := input
	ret.Payload = cloneMap(input.Payload)
	return ret
}
