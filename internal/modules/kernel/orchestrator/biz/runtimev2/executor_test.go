package runtimev2

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type failIfExecutedOperationExecutor struct {
	called bool
}

func (e *failIfExecutedOperationExecutor) ExecuteRuntimeOperation(context.Context, RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	e.called = true
	return RuntimeOperationExecutionResult{}, errors.New("operation must not be replayed")
}

type noOpOperationExecutor struct {
	owner *RuntimeController
}

func (e *noOpOperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeNoOp, ObservedEffectScope: runtimecontract.RuntimeEffectNone, NoOp: true,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "content already matched"},
	}
	_, committed, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{
		EventID: "event-" + request.Operation.ID, OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
		Status: ExecutionEvidenceStatusAccepted, SourceRef: "runtime://" + request.Operation.ID,
		Evidence: receipt.Evidence, Receipt: committed,
	}, nil
}

type receiptCommittedResponseLostExecutor struct {
	owner *RuntimeController
}

type acceptingOperationExecutor struct {
	owner  *RuntimeController
	called int
}

func (e *acceptingOperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	e.called++
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		Changed:  runtimecontract.RuntimeEffectIsDurable(request.Execution.ExpectedEffectScope),
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "accepted operation"},
	}
	_, committed, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{
		EventID: "event-" + request.Operation.ID, OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
		Status: ExecutionEvidenceStatusAccepted, SourceRef: "runtime://" + request.Operation.ID,
		Evidence: receipt.Evidence, Receipt: committed,
	}, nil
}

func (e *receiptCommittedResponseLostExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		Changed: true, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner committed before response loss"},
	}
	if _, _, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt); err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{}, errors.New("owner response lost after receipt commit")
}

type recordingOperationExecutor struct {
	requests []RuntimeOperationExecutionRequest
	rejectID string
	owner    *RuntimeController
}

func (e *recordingOperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	e.requests = append(e.requests, request)
	result := RuntimeOperationExecutionResult{
		EventID: "event-" + request.Operation.ID, OperationID: request.Operation.ID,
		EvidenceKind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
		SourceRef: "runtime://" + request.Operation.ID,
	}
	if request.Operation.ID == e.rejectID {
		result.Status = ExecutionEvidenceStatusRejected
		result.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "failure-" + request.Operation.ID, Message: "operation failed"}
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		Changed: true, Evidence: result.Evidence,
	}
	if result.Status == ExecutionEvidenceStatusRejected {
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectNone
		receipt.Changed = false
		receipt.Failure = result.Failure
	}
	if e.owner != nil {
		_, committed, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
		if err != nil {
			return RuntimeOperationExecutionResult{}, err
		}
		result.Receipt = committed
	}
	return result, nil
}

func TestRuntimeControllerExecutesDependencyOrderedOperationsAndCommitsEvidence(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: runtimecontract.StepKindFileEdit}}
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate := multiOperationCandidate()
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, candidate); err != nil {
		t.Fatal(err)
	}
	executor := &recordingOperationExecutor{owner: controller}
	completed, err := controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatalf("ExecuteActiveDispatch: %v", err)
	}
	if len(executor.requests) != 2 || executor.requests[0].Operation.ID != "write-a" || executor.requests[1].Operation.ID != "write-b" {
		t.Fatalf("execution order = %+v", executor.requests)
	}
	if len(executor.requests[0].EvidencePolicies) != 1 || executor.requests[0].EvidencePolicies[0].ID != "policy-1" {
		t.Fatalf("executor request lost typed evidence policy: %+v", executor.requests[0].EvidencePolicies)
	}
	if completed.ActiveDispatch != nil || len(completed.CompletedDispatches) != 1 || completed.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("completed aggregate = %+v", completed)
	}
	if completed.EvidenceSequence != 2 || len(completed.Facts.Evidence.Accepted) != 2 {
		t.Fatalf("evidence state = sequence %d facts %+v", completed.EvidenceSequence, completed.Facts.Evidence)
	}
}

func TestRuntimeControllerStopsDispatchAfterRejectedOperation(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, multiOperationCandidate()); err != nil {
		t.Fatal(err)
	}
	executor := &recordingOperationExecutor{rejectID: "write-a", owner: controller}
	failed, err := controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatalf("ExecuteActiveDispatch: %v", err)
	}
	if len(executor.requests) != 1 || failed.ActiveDispatch != nil || len(failed.CompletedDispatches) != 1 {
		t.Fatalf("failed dispatch = requests %+v aggregate %+v", executor.requests, failed)
	}
	if failed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || failed.Facts.Failure == nil {
		t.Fatalf("failed decision = %+v facts=%+v", failed.Decision, failed.Facts)
	}
}

func TestRuntimeControllerRecoversOwnerCommittedReceiptWithoutReplayingSideEffect(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("recovered")},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", "write-1")
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, "write-1")
	if !ok {
		t.Fatal("claimed execution is missing")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: "write-1", Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner committed before crash", EvidenceRefs: []string{"file://target.txt"}},
	}
	if _, _, err := controller.CommitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, receipt); err != nil {
		t.Fatal(err)
	}

	restarted, _ := NewRuntimeController(repository)
	executor := &failIfExecutedOperationExecutor{}
	completed, err := restarted.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called {
		t.Fatal("owner side effect was replayed after persisted receipt")
	}
	if len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 || completed.EvidenceSequence != 1 {
		t.Fatalf("receipt/evidence recovery is not exactly once: receipts=%d evidence=%d sequence=%d", len(completed.OperationReceipts), len(completed.Facts.Evidence.Accepted), completed.EvidenceSequence)
	}

	replayed, err := restarted.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err == nil || replayed.ActiveDispatch != nil {
		// A completed dispatch has no active execution to replay. The important
		// invariant is that the owner was not invoked and evidence stayed singular.
		if executor.called || len(completed.Facts.Evidence.Accepted) != 1 {
			t.Fatalf("restart replay changed exactly-once state: %+v err=%v", replayed, err)
		}
	}
}

func TestRuntimeControllerRecoversReceiptWhenOwnerResponseIsLostAfterCommit(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("recovered")},
	}}}}); err != nil {
		t.Fatal(err)
	}

	completed, err := controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", &receiptCommittedResponseLostExecutor{owner: controller})
	if err != nil {
		t.Fatalf("response-loss recovery: %v", err)
	}
	if completed.ActiveDispatch != nil || completed.Decision.Action != runtimecontract.RuntimeActionComplete ||
		len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("response-loss recovery aggregate = %+v", completed)
	}
	receipt := completed.OperationReceipts[0]
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.Fence == 0 {
		t.Fatalf("response-loss receipt = %+v", receipt)
	}
}

func TestRuntimeControllerTurnsClaimedWithoutReceiptIntoOutcomeUnknownWithoutReplay(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("unknown")},
	}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", "write-1"); err != nil {
		t.Fatal(err)
	}
	executor := &failIfExecutedOperationExecutor{}
	_, err = controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err == nil || !strings.Contains(err.Error(), "outcome_unknown") {
		t.Fatalf("claimed recovery err = %v, want outcome_unknown", err)
	}
	if executor.called {
		t.Fatal("claimed operation without receipt was blindly replayed")
	}
	loaded, _ := controller.Load(context.Background(), "ws-v2", "run-v2")
	execution, ok := operationExecutionRecord(loaded.ActiveDispatch.OperationExecutions, "write-1")
	if !ok || execution.State != runtimecontract.OperationExecutionOutcomeUnknown || len(loaded.OperationReceipts) != 0 || len(loaded.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("unknown recovery state = %+v aggregate=%+v", execution, loaded)
	}
}

func TestRuntimeControllerReplaysClaimedSafeOperationWithinRecoveryBudget(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindFileRead}}
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	path := "target.txt"
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "read-1", Kind: runtimecontract.StepKindFileRead, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{path}, Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &path}}},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", "read-1"); err != nil {
		t.Fatal(err)
	}
	restarted, _ := NewRuntimeController(repository)
	executor := &acceptingOperationExecutor{owner: restarted}
	completed, err := restarted.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called != 1 || completed.ActiveDispatch != nil || completed.Decision.Action != runtimecontract.RuntimeActionComplete ||
		completed.Facts.Recovery.OperationRecoveryAttempts != 1 || len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("safe claimed recovery sealed=%+v completed=%+v calls=%d", sealed, completed, executor.called)
	}
}

func TestRuntimeControllerRoutesDurableNoOpReceiptToStructuredSameFocusRepair(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("same")},
	}}}}); err != nil {
		t.Fatal(err)
	}
	progressed, err := controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", &noOpOperationExecutor{owner: controller})
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || progressed.Facts.Failure == nil || progressed.Facts.Failure.Class != runtimecontract.FailureClassEffectMismatch {
		t.Fatalf("no-op did not route to same-focus effect repair: decision=%+v failure=%+v", progressed.Decision, progressed.Facts.Failure)
	}
	if len(progressed.Facts.Evidence.Accepted) != 0 || len(progressed.Facts.Evidence.Rejected) != 1 || len(progressed.OperationReceipts) != 1 {
		t.Fatalf("no-op incorrectly closed durable evidence: accepted=%+v rejected=%+v receipts=%+v", progressed.Facts.Evidence.Accepted, progressed.Facts.Evidence.Rejected, progressed.OperationReceipts)
	}
}

func beginAndAcceptCandidate(t *testing.T, controller *RuntimeController, aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	t.Helper()
	if _, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
		return RuntimeAggregate{}, err
	}
	return controller.AcceptCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
}

func multiOperationCandidate() runtimecontract.RuntimeCandidate {
	contentA := "a"
	contentB := "b"
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: runtimecontract.RuntimeOperation{ID: "write-b", Kind: "file/write", DependsOn: []string{"write-a"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"b.txt"}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &contentB}}}},
		{Operation: runtimecontract.RuntimeOperation{ID: "write-a", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"a.txt"}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &contentA}}}},
	}}}
}
