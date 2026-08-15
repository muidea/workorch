package execution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/muidea/magicCommon/task"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workorchcommon "github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type recordingSubmitter struct {
	mu       sync.Mutex
	commands []workorchdevents.WorkspaceRunRequestedCommand
	contexts []context.Context
	result   workorchdevents.WorkspaceRunRequestedResult
	err      error
	done     chan struct{}
}

func (s *recordingSubmitter) SubmitWorkspaceRun(ctx context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	s.mu.Lock()
	s.commands = append(s.commands, command)
	s.contexts = append(s.contexts, ctx)
	if s.done != nil {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
	if s.err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, s.err
	}
	if s.result.WorkspaceID == "" {
		s.result = workorchdevents.WorkspaceRunRequestedResult{
			WorkspaceID: command.WorkspaceID,
			RequestID:   command.RequestID,
			Kind:        runtimecontract.RuntimeExecutionIntentKind(command.Intent),
			Outcome:     runtimecontract.RuntimeExecutionIntentReconciliationRequested,
			Reason:      runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
		}
	}
	return s.result, nil
}

type recordingProjection struct {
	mu              sync.Mutex
	queued          []workorchdevents.WorkspaceRunRequestedCommand
	outcomes        []workorchdevents.WorkspaceRunRequestedResult
	rejected        []error
	enqueueFailures []error
	queuedCh        chan struct{}
	outcomeCh       chan struct{}
	rejectedCh      chan struct{}
}

func (p *recordingProjection) RecordWorkspaceExecutionQueued(command workorchdevents.WorkspaceRunRequestedCommand) {
	p.mu.Lock()
	p.queued = append(p.queued, command)
	if p.queuedCh != nil {
		select {
		case p.queuedCh <- struct{}{}:
		default:
		}
	}
	p.mu.Unlock()
}

func (p *recordingProjection) RecordWorkspaceExecutionOutcome(_ string, _ workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	p.mu.Lock()
	p.outcomes = append(p.outcomes, outcome)
	if p.outcomeCh != nil {
		select {
		case p.outcomeCh <- struct{}{}:
		default:
		}
	}
	p.mu.Unlock()
}

func (p *recordingProjection) RecordWorkspaceExecutionRejected(_ string, _ workorchdevents.WorkspaceRunRequestedCommand, err error) {
	p.mu.Lock()
	p.rejected = append(p.rejected, err)
	if p.rejectedCh != nil {
		select {
		case p.rejectedCh <- struct{}{}:
		default:
		}
	}
	p.mu.Unlock()
}

func (p *recordingProjection) RecordWorkspaceExecutionEnqueueFailure(_ string, _ workorchdevents.WorkspaceRunRequestedCommand, err error) {
	p.mu.Lock()
	p.enqueueFailures = append(p.enqueueFailures, err)
	p.mu.Unlock()
}

func TestUseCaseEnqueuesWithStableIdentityAndScopedProcessContext(t *testing.T) {
	background := task.NewBackgroundRoutine(8)
	t.Cleanup(func() { background.Shutdown(context.Background()) })
	projection := &recordingProjection{queuedCh: make(chan struct{}, 1), outcomeCh: make(chan struct{}, 1)}
	submitter := &recordingSubmitter{done: make(chan struct{}, 1)}
	fixed := time.Unix(123, 456).UTC()
	useCase := NewWithClock(submitter, projection, background, func() time.Time { return fixed })

	type requestMarker struct{}
	requestCtx := context.WithValue(context.Background(), requestMarker{}, "must-not-leak")
	command, err := useCase.EnqueueWorkspaceExecution(requestCtx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: workorchcommon.WorkspaceID("ws-execution-usecase"),
		Intent:      "resume",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	wantRequestID := "workorchd-workspace_run-ws-execution-usecase-" + "123000000456"
	if command.RequestID != wantRequestID || command.Source != "http" {
		t.Fatalf("normalized command = %+v, want request=%q source=http", command, wantRequestID)
	}
	select {
	case <-projection.queuedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("queued projection was not recorded")
	}
	select {
	case <-projection.outcomeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("owner outcome was not projected")
	}

	submitter.mu.Lock()
	if len(submitter.contexts) != 1 {
		submitter.mu.Unlock()
		t.Fatalf("submitter calls = %d, want 1", len(submitter.contexts))
	}
	ownerCtx := submitter.contexts[0]
	requestValue := ownerCtx.Value(requestMarker{})
	scope := workorchcommon.RuntimeWorkspaceIDFromContext(ownerCtx)
	submitter.mu.Unlock()
	if requestValue != nil {
		t.Fatalf("request context value leaked into owner context: %v", requestValue)
	}
	if scope != workorchcommon.WorkspaceID("ws-execution-usecase") {
		t.Fatalf("owner workspace scope = %q", scope)
	}
	if err := useCase.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestUseCaseProjectsOwnerFailureWithoutRetry(t *testing.T) {
	background := task.NewBackgroundRoutine(8)
	t.Cleanup(func() { background.Shutdown(context.Background()) })
	projection := &recordingProjection{rejectedCh: make(chan struct{}, 1)}
	submitErr := errors.New("owner rejected intent")
	submitter := &recordingSubmitter{err: submitErr}
	useCase := New(submitter, projection, background)
	command := workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: "ws-owner-failure", Intent: "restart", RequestID: "req-owner-failure"}
	if _, err := useCase.EnqueueWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	select {
	case <-projection.rejectedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("owner rejection was not projected")
	}
	projection.mu.Lock()
	if len(projection.outcomes) != 0 || len(projection.rejected) != 1 || !errors.Is(projection.rejected[0], submitErr) {
		t.Fatalf("projection = outcomes=%d rejected=%v", len(projection.outcomes), projection.rejected)
	}
	projection.mu.Unlock()
	submitter.mu.Lock()
	callCount := len(submitter.commands)
	submitter.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("owner submit count = %d, want one bounded attempt", callCount)
	}
	if err := useCase.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestUseCaseSynchronousSubmissionUsesTypedOwnerOutcome(t *testing.T) {
	background := task.NewBackgroundRoutine(2)
	t.Cleanup(func() { background.Shutdown(context.Background()) })
	projection := &recordingProjection{outcomeCh: make(chan struct{}, 1)}
	submitter := &recordingSubmitter{}
	useCase := New(submitter, projection, background)
	ctx := workorchcommon.ContextWithRuntimeWorkspaceID(context.Background(), workorchcommon.WorkspaceID("ws-sync-admission"))
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-sync-admission",
		Intent:      "resume",
		RequestID:   "req-sync-admission",
		Source:      "cli",
	}
	outcome, err := useCase.SubmitWorkspaceExecution(ctx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if outcome.WorkspaceID != command.WorkspaceID || outcome.RequestID != command.RequestID || outcome.Kind != runtimecontract.RuntimeExecutionIntentResume {
		t.Fatalf("owner outcome = %+v", outcome)
	}
	select {
	case <-projection.outcomeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("synchronous outcome was not projected")
	}
	submitter.mu.Lock()
	if len(submitter.contexts) != 1 || workorchcommon.RuntimeWorkspaceIDFromContext(submitter.contexts[0]) != command.WorkspaceID {
		t.Fatalf("owner contexts = %v", submitter.contexts)
	}
	submitter.mu.Unlock()
	if err := useCase.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestUseCaseCoalescesDuplicateAsyncRequestIdentity(t *testing.T) {
	background := task.NewBackgroundRoutine(4)
	t.Cleanup(func() { background.Shutdown(context.Background()) })
	projection := &recordingProjection{outcomeCh: make(chan struct{}, 1)}
	submitter := &recordingSubmitter{}
	useCase := New(submitter, projection, background)
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-duplicate-admission",
		Intent:      "resume",
		RequestID:   "req-duplicate-admission",
		Source:      "http",
	}
	first, err := useCase.EnqueueWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := useCase.EnqueueWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command)
	if err != nil {
		t.Fatalf("duplicate enqueue: %v", err)
	}
	if first != second {
		t.Fatalf("duplicate command changed: first=%+v second=%+v", first, second)
	}
	select {
	case <-projection.outcomeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("owner outcome was not projected")
	}
	projection.mu.Lock()
	queued := len(projection.queued)
	projection.mu.Unlock()
	submitter.mu.Lock()
	submits := len(submitter.commands)
	submitter.mu.Unlock()
	if queued != 1 || submits != 1 {
		t.Fatalf("duplicate scheduling = queued %d submits %d, want one each", queued, submits)
	}
	conflicting := command
	conflicting.Intent = "restart"
	if _, err := useCase.EnqueueWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, conflicting); !errors.Is(err, workorchcommon.ErrConflict) {
		t.Fatalf("conflicting request reuse = %v, want ErrConflict", err)
	}
	if err := useCase.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestUseCaseFailsClosedWhenBackgroundQueueIsUnavailable(t *testing.T) {
	background := task.NewBackgroundRoutine(1)
	if !background.Shutdown(context.Background()) {
		t.Fatal("background shutdown failed")
	}
	projection := &recordingProjection{}
	submitter := &recordingSubmitter{}
	useCase := New(submitter, projection, background)
	_, err := useCase.EnqueueWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-queue-failure",
		Intent:      "resume",
		RequestID:   "req-queue-failure",
	})
	if !errors.Is(err, workorchcommon.ErrConflict) {
		t.Fatalf("enqueue error = %v, want ErrConflict", err)
	}
	projection.mu.Lock()
	if len(projection.enqueueFailures) != 1 || len(projection.queued) != 1 {
		t.Fatalf("queue failure projection = queued=%d failures=%d", len(projection.queued), len(projection.enqueueFailures))
	}
	projection.mu.Unlock()
	if err := useCase.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestUseCaseRejectsInvalidIntentBeforeQueue(t *testing.T) {
	background := task.NewBackgroundRoutine(1)
	t.Cleanup(func() { background.Shutdown(context.Background()) })
	projection := &recordingProjection{}
	submitter := &recordingSubmitter{}
	useCase := New(submitter, projection, background)
	_, err := useCase.EnqueueWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: workorchcommon.WorkspaceID("ws-invalid-intent"),
		Intent:      "pause",
		RequestID:   "req-invalid-intent",
	})
	if !errors.Is(err, workorchcommon.ErrInvalidArgument) {
		t.Fatalf("invalid intent error = %v, want ErrInvalidArgument", err)
	}
	projection.mu.Lock()
	queued := len(projection.queued)
	projection.mu.Unlock()
	if queued != 0 {
		t.Fatalf("queued invalid intent = %d, want zero", queued)
	}
	if err := useCase.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}
