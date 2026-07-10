package runtimeevent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Adapter interface {
	ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error)
}

var runtimeWorkerHeartbeatInterval = 30 * time.Second

type Executor struct {
	moduleID   string
	hub        magicevent.Hub
	observer   magicevent.SimpleObserver
	background task.BackgroundRoutine
	adapter    Adapter
	kinds      []string
	topics     []string

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
	CompletedData *CompletedData
	FailedData    *FailedData
}

func NewExecutor(moduleID string, hub magicevent.Hub, background task.BackgroundRoutine, adapter Adapter, kinds ...string) *Executor {
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
	if e.observer != nil {
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
		topic := RuntimeBlockExecuteTopic(kind)
		if e.observer == nil {
			e.observer = magicevent.NewSimpleObserverWithMatchID(e.moduleID, CommandDestination, e.hub)
		}
		e.observer.Subscribe(topic, e.handle)
		e.topics = append(e.topics, topic)
	}
	return nil
}

func (e *Executor) Teardown() {
	if e == nil {
		return
	}
	if e.observer != nil {
		for idx := len(e.topics) - 1; idx >= 0; idx-- {
			e.observer.Unsubscribe(e.topics[idx])
		}
	}
	e.observer = nil
	e.topics = nil
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
		if err := e.publishWorkerLifecycle(context.Background(), state.Request, state.RequestID, "runtime_worker_lost", "worker_lost", "runtime worker lost before terminal result", map[string]any{
			"queueKey": state.QueueKey,
			"workerId": state.WorkerID,
			"lostAt":   nowRFC3339(),
		}); err != nil {
			slog.Warn("runtime worker lost event publish failed", "module", e.moduleID, "requestID", state.RequestID, "queueKey", state.QueueKey, "err", err)
		}
	}
}

func (e *Executor) handle(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(ExecuteCommand)
	if !ok {
		setMagicError(result, common.ErrInvalidArgument)
		return
	}
	request := command.Request
	if request.Kind == "" {
		request.Kind = runtimeKindFromTopic(event.ID())
	}
	if request.Kind == "" || request.StepID == "" {
		setMagicError(result, common.ErrInvalidArgument)
		return
	}
	requestID := runtimeRequestID(command, request)
	queueKey := runtimeQueueKey(request, requestID)
	accepted := runtimemodel.RuntimeStepExecutionAccepted{
		WorkspaceID:    request.WorkspaceID,
		RunID:          request.RunID,
		StepID:         request.StepID,
		StepKind:       request.Kind,
		RequestID:      requestID,
		QueueKey:       queueKey,
		AcceptedAt:     nowRFC3339(),
		RecoveryPolicy: request.RecoveryPolicy.Normalized(),
	}
	schedule, replay, err := e.accept(queueKey, request, requestID)
	if err != nil {
		setMagicError(result, err)
		return
	}
	ctx := event.Context()
	if schedule {
		workerRequest := request
		workerRequest.Progress = e.progressPublisher(request, requestID)
		workerBaseCtx := context.Background()
		if ctx != nil {
			workerBaseCtx = context.WithoutCancel(ctx)
		}
		workerCtx, workerCancel := context.WithCancel(workerBaseCtx)
		startWorker := make(chan struct{})
		e.markWorkerCancel(queueKey, workerCancel)
		if err := e.background.AsyncFunction(func() {
			select {
			case <-startWorker:
			case <-workerCtx.Done():
				return
			}
			e.execute(workerCtx, workerRequest, requestID)
		}); err != nil {
			workerCancel()
			e.forget(queueKey)
			setMagicError(result, err)
			return
		}
		if err := e.publishAccepted(ctx, accepted); err != nil {
			workerCancel()
			e.forget(queueKey)
			setMagicError(result, err)
			return
		}
		close(startWorker)
	} else if !replay.empty() {
		if err := replay.publish(e.hub, ctx, e.moduleID); err != nil {
			setMagicError(result, err)
			return
		}
		if err := e.publishAccepted(ctx, accepted); err != nil {
			setMagicError(result, err)
			return
		}
	} else {
		if err := e.publishAccepted(ctx, accepted); err != nil {
			setMagicError(result, err)
			return
		}
	}
	setExecuteCommandResult(result, ExecuteCommandResult{Accepted: accepted})
}

type terminalNotification struct {
	Completed *CompletedData
	Failed    *FailedData
}

func (n terminalNotification) empty() bool {
	return n.Completed == nil && n.Failed == nil
}

func (n terminalNotification) publish(hub magicevent.Hub, ctx context.Context, source string) error {
	switch {
	case n.Completed != nil:
		return publishRuntimeNotification(hub, ctx, source, TopicRuntimeBlockExecuteCompleted, *n.Completed)
	case n.Failed != nil:
		return publishRuntimeNotification(hub, ctx, source, TopicRuntimeBlockExecuteFailed, *n.Failed)
	default:
		return common.ErrInvalidArgument
	}
}

func (e *Executor) accept(queueKey string, request runtimemodel.StepExecutionRequest, requestID string) (bool, terminalNotification, error) {
	if e == nil || queueKey == "" {
		return false, terminalNotification{}, common.ErrInvalidArgument
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return false, terminalNotification{}, fmt.Errorf("%w: runtime executor is closed", common.ErrInvalidArgument)
	}
	if e.requests == nil {
		e.requests = map[string]runtimeExecutionState{}
	}
	if state := e.requests[queueKey]; state.Accepted {
		if state.Terminal {
			return false, terminalNotification{Completed: cloneCompletedData(state.CompletedData), Failed: cloneFailedData(state.FailedData)}, nil
		}
		return false, terminalNotification{}, nil
	}
	e.requests[queueKey] = runtimeExecutionState{
		Accepted:  true,
		Request:   request,
		RequestID: requestID,
		QueueKey:  queueKey,
		WorkerID:  runtimeWorkerID(e.moduleID, requestID),
	}
	return true, terminalNotification{}, nil
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
	if err := e.publishWorkerLifecycle(ctx, request, requestID, "runtime_worker_started", "worker_started", "runtime worker started", map[string]any{
		"queueKey":  queueKey,
		"workerId":  workerID,
		"startedAt": startedAt,
	}); err != nil {
		slog.Warn("runtime worker started event publish failed", "module", e.moduleID, "requestID", requestID, "queueKey", queueKey, "err", err)
	}
	result, err := e.adapter.ExecuteRuntimeStep(ctx, request)
	if e.closedBeforeTerminal() && ctx.Err() != nil {
		if err := e.publishWorkerLifecycle(context.Background(), request, requestID, "runtime_worker_stopped", "worker_stopped", "runtime worker stopped after cancellation", map[string]any{
			"queueKey":  queueKey,
			"workerId":  workerID,
			"startedAt": startedAt,
			"stoppedAt": nowRFC3339(),
			"cancelled": true,
		}); err != nil {
			slog.Warn("runtime worker stopped event publish failed", "module", e.moduleID, "requestID", requestID, "queueKey", queueKey, "err", err)
		}
		return
	}
	if err != nil {
		if publishErr := e.publishFailed(ctx, request, requestID, result, err); publishErr != nil {
			slog.Warn("runtime failed event publish failed", "module", e.moduleID, "requestID", requestID, "queueKey", queueKey, "err", publishErr)
			return
		}
		if err := e.publishWorkerLifecycle(ctx, request, requestID, "runtime_worker_stopped", "worker_stopped", "runtime worker stopped", map[string]any{
			"queueKey":  queueKey,
			"workerId":  workerID,
			"startedAt": startedAt,
			"stoppedAt": nowRFC3339(),
		}); err != nil {
			slog.Warn("runtime worker stopped event publish failed", "module", e.moduleID, "requestID", requestID, "queueKey", queueKey, "err", err)
		}
		return
	}
	if err := e.publishCompleted(ctx, request, requestID, result); err != nil {
		slog.Warn("runtime completed event publish failed", "module", e.moduleID, "requestID", requestID, "queueKey", queueKey, "err", err)
		return
	}
	if err := e.publishWorkerLifecycle(ctx, request, requestID, "runtime_worker_stopped", "worker_stopped", "runtime worker stopped", map[string]any{
		"queueKey":  queueKey,
		"workerId":  workerID,
		"startedAt": startedAt,
		"stoppedAt": nowRFC3339(),
	}); err != nil {
		slog.Warn("runtime worker stopped event publish failed", "module", e.moduleID, "requestID", requestID, "queueKey", queueKey, "err", err)
	}
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
	if err := e.background.Timer(ctx, runtimeWorkerHeartbeatTask{fn: func() {
		if err := e.publishWorkerLifecycle(context.Background(), request, requestID, "runtime_worker_heartbeat", "worker_heartbeat", "runtime worker heartbeat", map[string]any{
			"queueKey":     queueKey,
			"workerId":     workerID,
			"heartbeatAt":  nowRFC3339(),
			"heartbeatTTL": runtimeWorkerHeartbeatInterval.String(),
		}); err != nil {
			slog.Warn("runtime worker heartbeat event publish failed", "module", e.moduleID, "requestID", requestID, "queueKey", queueKey, "err", err)
		}
	}}, runtimeWorkerHeartbeatInterval, 0); err != nil {
		recordRuntimeHeartbeatTimerStartFailure()
		slog.Warn("runtime worker heartbeat timer start failed", "module", e.moduleID, "requestID", requestID, "queueKey", queueKey, "err", err)
	}
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
		WorkspaceID:    request.WorkspaceID,
		RunID:          request.RunID,
		StepID:         request.StepID,
		StepKind:       request.Kind,
		RequestID:      requestID,
		Event:          event,
		Stage:          stage,
		Summary:        summary,
		Data:           cloneMap(data),
		OccurredAt:     nowRFC3339(),
		RecoveryPolicy: request.RecoveryPolicy.Normalized(),
	}
	return publishRuntimeNotification(e.hub, ctx, e.moduleID, TopicRuntimeBlockExecuteProgress, ProgressData{Progress: progress})
}

func (e *Executor) progressPublisher(request runtimemodel.StepExecutionRequest, requestID string) runtimemodel.StepExecutionProgressFunc {
	return func(progress runtimemodel.StepExecutionProgress) error {
		payload := runtimemodel.RuntimeStepExecutionProgressed{
			WorkspaceID:    request.WorkspaceID,
			RunID:          request.RunID,
			StepID:         request.StepID,
			StepKind:       request.Kind,
			RequestID:      requestID,
			Event:          progress.Event,
			Stage:          progress.Stage,
			Summary:        progress.Summary,
			Data:           cloneMap(progress.Data),
			OccurredAt:     nowRFC3339(),
			RecoveryPolicy: request.RecoveryPolicy.Normalized(),
		}
		if err := publishRuntimeNotification(e.hub, context.Background(), e.moduleID, TopicRuntimeBlockExecuteProgress, ProgressData{Progress: payload}); err != nil {
			return err
		}
		return nil
	}
}

func (e *Executor) publishAccepted(ctx context.Context, accepted runtimemodel.RuntimeStepExecutionAccepted) error {
	return publishRuntimeNotification(e.hub, ctx, e.moduleID, TopicRuntimeBlockExecuteAccepted, AcceptedData{Accepted: accepted})
}

func (e *Executor) publishCompleted(ctx context.Context, request runtimemodel.StepExecutionRequest, requestID string, result runtimemodel.StepExecutionResult) error {
	completed := runtimemodel.RuntimeStepExecutionCompleted{
		WorkspaceID:    request.WorkspaceID,
		RunID:          request.RunID,
		StepID:         request.StepID,
		StepKind:       request.Kind,
		RequestID:      requestID,
		Output:         cloneMap(result.Output),
		CompletedAt:    nowRFC3339(),
		RecoveryPolicy: request.RecoveryPolicy.Normalized(),
	}
	data := CompletedData{Completed: completed, Result: result}
	if err := publishRuntimeNotification(e.hub, ctx, e.moduleID, TopicRuntimeBlockExecuteCompleted, data); err != nil {
		return err
	}
	e.recordCompleted(request, requestID, data)
	return nil
}

func (e *Executor) publishFailed(ctx context.Context, request runtimemodel.StepExecutionRequest, requestID string, result runtimemodel.StepExecutionResult, execErr error) error {
	failed := runtimemodel.RuntimeStepExecutionFailed{
		WorkspaceID:    request.WorkspaceID,
		RunID:          request.RunID,
		StepID:         request.StepID,
		StepKind:       request.Kind,
		RequestID:      requestID,
		Error:          execErr.Error(),
		CompletedAt:    nowRFC3339(),
		RawResultRef:   "",
		RecoveryPolicy: request.RecoveryPolicy.Normalized(),
	}
	data := FailedData{Failed: failed, Result: result, Error: execErr.Error()}
	if err := publishRuntimeNotification(e.hub, ctx, e.moduleID, TopicRuntimeBlockExecuteFailed, data); err != nil {
		return err
	}
	e.recordFailed(request, requestID, data)
	return nil
}

func (e *Executor) recordCompleted(request runtimemodel.StepExecutionRequest, requestID string, data CompletedData) {
	e.recordTerminal(request, requestID, &data, nil)
}

func (e *Executor) recordFailed(request runtimemodel.StepExecutionRequest, requestID string, data FailedData) {
	e.recordTerminal(request, requestID, nil, &data)
}

func (e *Executor) recordTerminal(request runtimemodel.StepExecutionRequest, requestID string, completed *CompletedData, failed *FailedData) {
	if e == nil {
		return
	}
	queueKey := runtimeQueueKey(request, requestID)
	if queueKey == "" || (completed == nil && failed == nil) {
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
	state.CompletedData = cloneCompletedData(completed)
	state.FailedData = cloneFailedData(failed)
	e.requests[queueKey] = state
}

type runtimeNotificationData interface {
	AcceptedData | ProgressData | CompletedData | FailedData
}

func publishRuntimeNotification[Data runtimeNotificationData](hub magicevent.Hub, ctx context.Context, source string, topic string, data Data) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(topic, source, NotificationDestination, magicevent.NewHeader(), ctx, data)
	hub.Post(event)
	return nil
}

func runtimeRequestID(command ExecuteCommand, request runtimemodel.StepExecutionRequest) string {
	if value := strings.TrimSpace(command.RequestID); value != "" {
		return value
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

func setMagicError(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	if err == nil {
		err = common.ErrInvalidArgument
	}
	result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
}

func setExecuteCommandResult(result magicevent.Result, data ExecuteCommandResult) {
	if result != nil {
		result.Set(data, nil)
	}
}

func cloneCompletedData(data *CompletedData) *CompletedData {
	if data == nil {
		return nil
	}
	ret := *data
	return &ret
}

func cloneFailedData(data *FailedData) *FailedData {
	if data == nil {
		return nil
	}
	ret := *data
	return &ret
}
