package execution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
}

func (s *recordingSubmitter) SubmitWorkspaceRun(ctx context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	s.mu.Lock()
	s.commands = append(s.commands, command)
	s.contexts = append(s.contexts, ctx)
	result, err := s.result, s.err
	s.mu.Unlock()
	if err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	if result.WorkspaceID == "" {
		result = workorchdevents.WorkspaceRunRequestedResult{
			WorkspaceID: command.WorkspaceID,
			RequestID:   command.RequestID,
			Kind:        runtimecontract.RuntimeExecutionIntentKind(command.Intent),
			Outcome:     runtimecontract.RuntimeExecutionIntentReconciliationRequested,
			Reason:      runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
		}
	}
	return result, nil
}

type recordingProjection struct {
	mu       sync.Mutex
	outcomes []workorchdevents.WorkspaceRunRequestedResult
	rejected []error
}

func (p *recordingProjection) RecordWorkspaceExecutionOutcome(_ string, _ workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outcomes = append(p.outcomes, outcome)
}

func (p *recordingProjection) RecordWorkspaceExecutionRejected(_ string, _ workorchdevents.WorkspaceRunRequestedCommand, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rejected = append(p.rejected, err)
}

func TestAdmitWorkspaceExecutionWaitsForScopedOwnerAdmission(t *testing.T) {
	projection := &recordingProjection{}
	submitter := &recordingSubmitter{}
	fixed := time.Unix(123, 456).UTC()
	useCase := NewWithClock(submitter, projection, func() time.Time { return fixed })

	type requestMarker struct{}
	requestCtx := context.WithValue(context.Background(), requestMarker{}, "request-live")
	command, err := useCase.AdmitWorkspaceExecution(requestCtx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: workorchcommon.WorkspaceID("ws-execution-usecase"),
		Intent:      "resume",
	})
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	wantRequestID := "workorchd-workspace_run-ws-execution-usecase-123000000456"
	if command.RequestID != wantRequestID || command.Source != "http" {
		t.Fatalf("normalized command = %+v, want request=%q source=http", command, wantRequestID)
	}

	submitter.mu.Lock()
	if len(submitter.contexts) != 1 {
		submitter.mu.Unlock()
		t.Fatalf("submitter calls = %d, want 1", len(submitter.contexts))
	}
	ownerCtx := submitter.contexts[0]
	submitter.mu.Unlock()
	if scope := workorchcommon.RuntimeWorkspaceIDFromContext(ownerCtx); scope != command.WorkspaceID {
		t.Fatalf("owner workspace scope = %q", scope)
	}
	if marker := ownerCtx.Value(requestMarker{}); marker != "request-live" {
		t.Fatalf("synchronous request context marker = %v", marker)
	}
	projection.mu.Lock()
	defer projection.mu.Unlock()
	if len(projection.outcomes) != 1 || len(projection.rejected) != 0 {
		t.Fatalf("projection outcomes=%d rejected=%d", len(projection.outcomes), len(projection.rejected))
	}
}

func TestAdmitWorkspaceExecutionReturnsOwnerFailureSynchronouslyWithoutRetry(t *testing.T) {
	projection := &recordingProjection{}
	submitErr := errors.New("owner rejected intent")
	submitter := &recordingSubmitter{err: submitErr}
	useCase := New(submitter, projection)
	command := workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: "ws-owner-failure", Intent: "restart", RequestID: "req-owner-failure"}
	if _, err := useCase.AdmitWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command); !errors.Is(err, submitErr) {
		t.Fatalf("admission error = %v, want owner error", err)
	}
	submitter.mu.Lock()
	callCount := len(submitter.commands)
	submitter.mu.Unlock()
	projection.mu.Lock()
	rejected := append([]error(nil), projection.rejected...)
	outcomeCount := len(projection.outcomes)
	projection.mu.Unlock()
	if callCount != 1 || outcomeCount != 0 || len(rejected) != 1 || !errors.Is(rejected[0], submitErr) {
		t.Fatalf("calls=%d outcomes=%d rejected=%v", callCount, outcomeCount, rejected)
	}
}

func TestDuplicateAdmissionIsDelegatedToWorkspaceOwner(t *testing.T) {
	projection := &recordingProjection{}
	submitter := &recordingSubmitter{}
	useCase := New(submitter, projection)
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-owner-idempotency", Intent: "resume", RequestID: "req-owner-idempotency", Source: "http",
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := useCase.AdmitWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command); err != nil {
			t.Fatalf("admission %d: %v", attempt, err)
		}
	}
	submitter.mu.Lock()
	defer submitter.mu.Unlock()
	if len(submitter.commands) != 2 {
		t.Fatalf("owner submissions = %d, want both requests delegated", len(submitter.commands))
	}
}

func TestSubmitWorkspaceExecutionReturnsTypedOwnerOutcome(t *testing.T) {
	projection := &recordingProjection{}
	submitter := &recordingSubmitter{}
	useCase := New(submitter, projection)
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-sync-admission", Intent: "resume", RequestID: "req-sync-admission", Source: "cli",
	}
	outcome, err := useCase.SubmitWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if outcome.WorkspaceID != command.WorkspaceID || outcome.RequestID != command.RequestID || outcome.Kind != runtimecontract.RuntimeExecutionIntentResume {
		t.Fatalf("owner outcome = %+v", outcome)
	}
}

func TestUseCaseRejectsInvalidIntentBeforeOwnerAdmission(t *testing.T) {
	projection := &recordingProjection{}
	submitter := &recordingSubmitter{}
	useCase := New(submitter, projection)
	_, err := useCase.AdmitWorkspaceExecution(context.Background(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-invalid-intent", Intent: "pause", RequestID: "req-invalid-intent",
	})
	if !errors.Is(err, workorchcommon.ErrInvalidArgument) {
		t.Fatalf("invalid intent error = %v, want ErrInvalidArgument", err)
	}
	submitter.mu.Lock()
	defer submitter.mu.Unlock()
	if len(submitter.commands) != 0 {
		t.Fatalf("owner calls = %d, want zero", len(submitter.commands))
	}
}

func TestValidateOwnerOutcomeRejectsAdmissionScopeMismatch(t *testing.T) {
	command := workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: "ws-owner-outcome-scope", Intent: "resume", RequestID: "req-owner-outcome-scope"}
	valid := workorchdevents.WorkspaceRunRequestedResult{
		WorkspaceID: command.WorkspaceID,
		RequestID:   command.RequestID,
		Kind:        runtimecontract.RuntimeExecutionIntentResume,
		Outcome:     runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		Reason:      runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
	}
	if err := validateOwnerOutcome(command, valid); err != nil {
		t.Fatalf("valid owner outcome: %v", err)
	}
	invalid := valid
	invalid.RequestID = "req-other"
	if err := validateOwnerOutcome(command, invalid); !errors.Is(err, workorchcommon.ErrConflict) {
		t.Fatalf("scope mismatch error = %v, want ErrConflict", err)
	}
}
