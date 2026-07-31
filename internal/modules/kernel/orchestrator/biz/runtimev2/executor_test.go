package runtimev2

import (
	"context"
	"errors"
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
		Outcome: runtimecontract.RuntimeOperationOutcomeNoOp, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "content already matched"},
	}
	_, committed, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{
		EventID: "event-" + string(request.WorkspaceID) + "-" + string(request.RunID) + "-" + request.Operation.ID, OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
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

// replayProofRecoveryExecutor models an owner whose durable side effect
// completed but whose receipt CAS failed. The second invocation is only valid
// when the controller preserves the original execution key/fence and the
// sealed owner proof.
type replayProofRecoveryExecutor struct {
	owner    *RuntimeController
	requests []RuntimeOperationExecutionRequest
}

func (e *replayProofRecoveryExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	e.requests = append(e.requests, request)
	if len(e.requests) == 1 {
		return RuntimeOperationExecutionResult{}, errors.New("durable side effect completed before receipt CAS loss")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		Changed: true, BeforeHash: "before", AfterHash: "after",
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...),
		Evidence:   runtimecontract.RuntimeOperationEvidence{Summary: "owner replay proof confirmed"},
	}
	_, committed, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{
		EventID: "event-" + request.Operation.ID, OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
		Status: ExecutionEvidenceStatusAccepted, SourceRef: "runtime://" + request.Operation.ID,
		Evidence: committed.Evidence, Receipt: committed,
	}, nil
}

type qualityOperationExecutor struct {
	owner    *RuntimeController
	evidence runtimecontract.RuntimeOperationEvidence
	called   int
}

type sealedValidationRecipeExecutor struct {
	owner    *RuntimeController
	requests []RuntimeOperationExecutionRequest
}

func (e *sealedValidationRecipeExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	e.requests = append(e.requests, request)
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: "sealed validation recipe executed",
			Validation: &runtimecontract.RuntimeValidationObservation{
				ExecutedChecks: []string{"sealed-recipe"}, PassedChecks: []string{"sealed-recipe"},
			},
		},
	}
	_, committed, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{
		EventID: "event-" + request.Operation.ID, OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
		Status: ExecutionEvidenceStatusAccepted, SourceRef: "runtime://" + request.Operation.ID,
		Evidence: committed.Evidence, Receipt: committed,
	}, nil
}

func (e *qualityOperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	e.called++
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...),
		Evidence:   e.evidence,
	}
	_, committed, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{
		EventID: "event-" + request.Operation.ID, OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
		Status: ExecutionEvidenceStatusAccepted, SourceRef: "runtime://" + request.Operation.ID,
		Evidence: committed.Evidence, Receipt: committed,
	}, nil
}

func (e *acceptingOperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	e.called++
	evidence := runtimecontract.RuntimeOperationEvidence{Summary: "accepted operation"}
	if runtimecontract.CanonicalRuntimeBlockKind(request.Operation.Kind) == runtimecontract.RuntimeBlockKindManagedValidationSession && len(request.ValidationRecipes) == 1 {
		evidence.Validation = &runtimecontract.RuntimeValidationObservation{RecipeID: request.ValidationRecipes[0].ID}
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		Changed: runtimecontract.RuntimeEffectIsDurable(request.Execution.ExpectedEffectScope),
		// C12 fixture receipts must report the target actually observed by the
		// owner; a sealed operation target is not itself receipt evidence.
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...),
		Evidence:   evidence,
	}
	_, committed, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{
		EventID:     "event-" + string(request.WorkspaceID) + "-" + string(request.RunID) + "-" + request.Operation.ID,
		OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
		Status: ExecutionEvidenceStatusAccepted, SourceRef: "runtime://" + request.Operation.ID,
		Evidence: receipt.Evidence, Receipt: committed,
	}, nil
}

func (e *receiptCommittedResponseLostExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		Changed: true, TargetRefs: append([]string(nil), request.Operation.TargetRefs...),
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner committed before response loss"},
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
		result.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Code: "operation_failed", Signature: "failure-" + request.Operation.ID, Message: "operation failed"}
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		Changed: true, TargetRefs: append([]string(nil), request.Operation.TargetRefs...), Evidence: result.Evidence,
	}
	if result.Status == ExecutionEvidenceStatusRejected {
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectNone
		receipt.Changed = false
		receipt.Failure = result.Failure
		descriptor, _ := runtimecontract.RuntimeOperationDescriptorForKind(request.Operation.Kind)
		feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
			WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID, DispatchID: request.DispatchID,
		}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(receipt.Outcome), descriptor.OwnerModuleID, *receipt.Failure)
		if err != nil {
			return RuntimeOperationExecutionResult{}, err
		}
		feedback.OriginalDiagnostic.OperationID = request.Operation.ID
		receipt.Feedback = &feedback
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
	// This test covers dispatch ordering. Recompile the fixture's declaration
	// as an owner selector so its a.txt/b.txt writes do not borrow the default
	// exact target.txt binding.
	facts.Focus.TargetRefs = nil
	facts = runtimeV2TestFactsWithCurrentFocusBindings(t, facts, controllerDefinition())
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
	if completed.ActiveDispatch != nil || len(completed.CompletedDispatches) != 1 || completed.Decision.State != runtimecontract.ConvergenceStateComplete {
		t.Fatalf("completed aggregate = %+v", completed)
	}
	if completed.EvidenceSequence != 2 || len(completed.Facts.Evidence.Accepted) != 2 {
		t.Fatalf("evidence state = sequence %d facts %+v", completed.EvidenceSequence, completed.Facts.Evidence)
	}
}

func TestRuntimeControllerRestartContinuesSealedDispatchAfterResourceChangingReceipt(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	_, _ = addFreshFileAdmissibilityObservation(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: patchApplyTestOperation("write-1", "main.go")},
		{Operation: func() runtimecontract.RuntimeOperation {
			operation := patchApplyTestOperation("write-2", "other.go")
			// The exact main.go binding belongs only to write-1. This second
			// operation is intentionally non-crediting dispatch work.
			operation.BindingRefs = nil
			operation.DependsOn = []string{"write-1"}
			return operation
		}()},
	}}}
	sealed := sealCandidateForTest(t, repository, candidate)
	claimed, err := controller.claimOperationExecution(ctx, "ws-1", "run-1", "write-1")
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, "write-1")
	if !ok {
		t.Fatal("first operation execution was not claimed")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: "write-1", Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
		TargetRefs: []string{"main.go"},
		Evidence:   runtimecontract.RuntimeOperationEvidence{Summary: "first durable effect"},
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2",
		}},
	}
	if _, _, err := controller.CommitOperationReceipt(ctx, "ws-1", "run-1", sealed.ActiveDispatch.ID, execution.Fence, receipt); err != nil {
		t.Fatal(err)
	}

	// Simulate a process loss after the owner receipt CAS and before evidence
	// commit. The restarted SM-3 owner must commit that receipt exactly once and
	// execute only the remaining sealed operation; it must not re-request a
	// candidate or replay the first durable effect.
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingOperationExecutor{owner: restarted}
	completed, err := restarted.ExecuteActiveDispatch(ctx, "ws-1", "run-1", executor)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.requests) != 1 || executor.requests[0].Operation.ID != "write-2" ||
		completed.ActiveDispatch != nil || len(completed.CompletedDispatches) != 1 ||
		completed.CompletedDispatches[0].State != DispatchStateCommitted ||
		completed.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		len(completed.Facts.Evidence.Accepted) != 2 || len(completed.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("restart did not continue the sealed dispatch correctly: requests=%+v aggregate=%+v", executor.requests, completed)
	}
}

func TestRuntimeControllerStopsDispatchAfterRejectedOperation(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	// This test stops a rejected operation before it can close coverage.
	// Recompile an owner-selector binding for its a.txt/b.txt fixture targets.
	facts.Focus.TargetRefs = nil
	facts = runtimeV2TestFactsWithCurrentFocusBindings(t, facts, controllerDefinition())
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
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
	if failed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || failed.Facts.Failure == nil {
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
		TargetRefs: []string{"target.txt"},
		Evidence:   runtimecontract.RuntimeOperationEvidence{Summary: "owner committed before crash", EvidenceRefs: []string{"file://target.txt"}},
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
	if completed.ActiveDispatch != nil || completed.Decision.State != runtimecontract.ConvergenceStateComplete ||
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
	var outcomeUnknown *OperationOutcomeUnknownError
	if !errors.As(err, &outcomeUnknown) {
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
	facts := runtimeV2FactsWithManagedValidationRecipe(controllerFacts())
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}}
	facts = runtimeV2TestFactsWithCurrentFocusBindings(t, facts, controllerDefinition())
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, managedValidationCandidate("read-1"))
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
	if executor.called != 1 || completed.ActiveDispatch != nil || completed.Decision.State != runtimecontract.ConvergenceStateComplete ||
		completed.Facts.Recovery.OperationRecoveryAttempts != 1 || len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("safe claimed recovery sealed=%+v completed=%+v calls=%d", sealed, completed, executor.called)
	}
}

func TestRuntimeControllerRecoversClaimedAtMostOnceOperationOnlyWithSealedReplayProof(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Capabilities = runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}}
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindPatchApply}
	facts = runtimeV2TestFactsWithCurrentFocusBindings(t, facts, definition)
	_, err := controller.Initialize(context.Background(), facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	patchType, target, content := "file_write", "target.txt", "after\n"
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}},
	}}}}}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AdmitCandidateProposalForTest(context.Background(), "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	proof := runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationID: "patch-1", TargetRefs: []string{target},
		BeforeHash: "before", AfterHash: "after", AdmissibilitySnapshotID: "admissibility-1",
	}
	if _, err := controller.RecordCandidateFeasibility(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: "patch-1", AdmissibilitySnapshotID: "admissibility-1", Feasible: true, ReplayProof: &proof,
	}); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AdmitAndSealCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("seal replay-proof candidate aggregate=%+v err=%v", sealed.Aggregate, err)
	}
	execution, ok := operationExecutionRecord(sealed.Aggregate.ActiveDispatch.OperationExecutions, "patch-1")
	if !ok || execution.ReplayProof == nil || execution.ReplayProof.AfterHash != "after" {
		t.Fatalf("sealed execution omitted replay proof: %+v", execution)
	}
	executor := &replayProofRecoveryExecutor{owner: controller}
	completed, err := controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.requests) != 2 || executor.requests[0].Execution.ExecutionKey != executor.requests[1].Execution.ExecutionKey || executor.requests[0].Execution.Fence != executor.requests[1].Execution.Fence || executor.requests[1].Execution.ReplayProof == nil {
		t.Fatalf("recovery did not reuse sealed execution identity/proof: %+v", executor.requests)
	}
	if completed.ActiveDispatch != nil || completed.Decision.State != runtimecontract.ConvergenceStateComplete || completed.Facts.Recovery.OperationRecoveryAttempts != 1 || len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("replay-proof recovery did not converge exactly once: %+v", completed)
	}
}

func TestRuntimeControllerRoutesStateChangeNoOpToOneBoundedCurrentStateValidation(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Capabilities.ValidationKinds = []string{runtimecontract.StepKindFileRead}
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
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
	if progressed.Decision.State != runtimecontract.ConvergenceStateNeedValidation || progressed.Facts.Failure == nil || progressed.Facts.Failure.Class != runtimecontract.FailureClassEffectMismatch || progressed.Facts.Failure.Code != runtimeStateChangeNoOpWithoutProvenanceCode {
		t.Fatalf("no-op did not route to bounded current-state validation: decision=%+v failure=%+v", progressed.Decision, progressed.Facts.Failure)
	}
	if len(progressed.Facts.Failure.TargetRefs) != 0 || progressed.Facts.Resources.FailureTarget != nil {
		t.Fatalf("core effect mismatch promoted operation input into repair authority: failure=%+v failureTarget=%+v", progressed.Facts.Failure, progressed.Facts.Resources.FailureTarget)
	}
	if len(progressed.Facts.Repair.NoProgressOperationShapes) != 1 || progressed.Facts.Repair.NoProgressOperationShapes[0].FailureCode != runtimeStateChangeNoOpWithoutProvenanceCode || progressed.Facts.Repair.NoProgressCount != 1 {
		t.Fatalf("no-op did not persist exact bounded no-progress lineage: %+v", progressed.Facts.Repair)
	}
	if len(progressed.Facts.Evidence.Accepted) != 0 || len(progressed.Facts.Evidence.Rejected) != 1 || len(progressed.OperationReceipts) != 1 {
		t.Fatalf("no-op incorrectly closed durable evidence: accepted=%+v rejected=%+v receipts=%+v", progressed.Facts.Evidence.Accepted, progressed.Facts.Evidence.Rejected, progressed.OperationReceipts)
	}
	if _, err := beginAndAcceptCandidate(t, controller, progressed, runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "inspect-current-state", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{"target.txt"}},
	}}}}); err != nil {
		t.Fatal(err)
	}
	inspected, err := controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", &noOpOperationExecutor{owner: controller})
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Decision.State != runtimecontract.ConvergenceStateFailed || inspected.Decision.Terminal == nil || inspected.Decision.Terminal.Code != "state_change_provenance_missing" {
		t.Fatalf("validated no-op state did not terminate the provenance gap: decision=%+v", inspected.Decision)
	}
	if len(inspected.Facts.Evidence.Accepted) != 1 || inspected.Facts.Evidence.Accepted[0].Kind != runtimecontract.EvidenceKindValidation || len(inspected.Facts.Evidence.Rejected) != 1 {
		t.Fatalf("no-op path fabricated state-change evidence: accepted=%+v rejected=%+v", inspected.Facts.Evidence.Accepted, inspected.Facts.Evidence.Rejected)
	}
}

func TestRuntimeControllerRestartAtomicallyInvalidatesUncommittableOwnerReceipt(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("content")},
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
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner observed no durable change"},
	}
	if _, _, err := controller.CommitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, receipt); err != nil {
		t.Fatal(err)
	}

	restarted, _ := NewRuntimeController(repository)
	executor := &failIfExecutedOperationExecutor{}
	progressed, err := restarted.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called || progressed.ActiveDispatch != nil || len(progressed.CompletedDispatches) != 1 ||
		progressed.CompletedDispatches[0].State != DispatchStateInvalidated {
		t.Fatalf("uncommittable receipt did not atomically close the dispatch: %+v called=%v", progressed, executor.called)
	}
	if progressed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || progressed.Facts.Failure == nil ||
		progressed.Facts.Failure.Class != runtimecontract.FailureClassEffectMismatch ||
		len(progressed.Facts.Evidence.Accepted) != 0 || len(progressed.Facts.Evidence.Rejected) != 1 {
		t.Fatalf("uncommittable receipt did not atomically reduce SM-2: decision=%+v failure=%+v evidence=%+v", progressed.Decision, progressed.Facts.Failure, progressed.Facts.Evidence)
	}
}

func TestRuntimeControllerRestartMigratesPersistedMixedEffectDispatchWithoutReplay(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("content")},
	}}}})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal aggregate=%+v err=%v", sealed, err)
	}
	// Persist the exact pre-C1 shape: a non-durable Shell operation was sealed
	// alongside a durable File operation under produce_state_change, and its
	// accepted owner receipt was durable before the process restarted.
	falseFlag := false
	command := "go"
	shell := runtimecontract.RuntimeOperation{
		ID: "a-validate", Kind: runtimecontract.StepKindShellCommand, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "command", String: &command}, {Name: "args", StringList: []string{"test", "./..."}},
			{Name: "stateChanging", Boolean: &falseFlag},
		},
	}
	dispatch := repository.aggregate.ActiveDispatch
	dispatch.Candidate.StateChange.Operations = append(dispatch.Candidate.StateChange.Operations, runtimecontract.StateChangingOperation{Operation: shell})
	dispatch.CandidateHash = runtimecontract.StableRuntimeCandidateHash(dispatch.Candidate)
	executionKey := runtimecontract.StableRuntimeOperationExecutionKey("ws-v2", "run-v2", dispatch.ID, shell.ID)
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: executionKey, Fence: 1, OperationID: shell.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "validation completed"},
	}
	dispatch.OperationExecutions = append(dispatch.OperationExecutions, runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: shell.ID, ExecutionKey: executionKey, Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
		State: runtimecontract.OperationExecutionOwnerCommitted, OwnerReceiptRef: "runtime-receipt/" + executionKey,
		OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt),
	})
	repository.aggregate.OperationReceipts = append(repository.aggregate.OperationReceipts, receipt)

	restarted, _ := NewRuntimeController(repository)
	executor := &failIfExecutedOperationExecutor{}
	progressed, err := restarted.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called || progressed.ActiveDispatch != nil || len(progressed.CompletedDispatches) != 1 ||
		progressed.CompletedDispatches[0].State != DispatchStateInvalidated || progressed.Facts.Failure == nil ||
		progressed.Facts.Failure.Class != runtimecontract.FailureClassEffectMismatch {
		t.Fatalf("persisted mixed-effect dispatch did not migrate atomically: %+v called=%v", progressed, executor.called)
	}
}

func TestRecordFailureFactRejectsActiveDispatch(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("content")},
	}}}})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal aggregate=%+v err=%v", sealed, err)
	}
	revision := repository.aggregate.StateRevision
	_, err = controller.RecordFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeInvariantDefect, Code: "generic", Signature: "generic", Message: "generic failure",
	})
	if !errors.Is(err, common.ErrConflict) || repository.aggregate.StateRevision != revision || repository.aggregate.ActiveDispatch == nil {
		t.Fatalf("generic failure changed an active dispatch: aggregate=%+v err=%v", repository.aggregate, err)
	}
}

func TestRecordDriveFailureFactAtomicallyInvalidatesActiveDispatch(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("content")},
	}}}})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal aggregate=%+v err=%v", sealed, err)
	}

	failed, err := controller.RecordDriveFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_v2_drive_failed",
		Signature: "runtime-v2-drive-failed", Message: "typed drive contract failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.ActiveDispatch != nil || len(failed.CompletedDispatches) != 1 ||
		failed.CompletedDispatches[0].ID != sealed.ActiveDispatch.ID || failed.CompletedDispatches[0].State != DispatchStateInvalidated {
		t.Fatalf("drive failure did not atomically invalidate dispatch: %+v", failed)
	}
	if failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Decision.Terminal == nil ||
		failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassRuntimeContractMisalignment {
		t.Fatalf("drive failure did not atomically reduce SM-2: %+v", failed)
	}
	saves := repository.saves
	replayed, err := controller.RecordDriveFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Message: "duplicate drive failure",
	})
	if err != nil || repository.saves != saves || replayed.StateRevision != failed.StateRevision {
		t.Fatalf("duplicate terminal drive failure changed state: aggregate=%+v saves=%d want=%d err=%v", replayed, repository.saves, saves, err)
	}
}

func TestRuntimeControllerRejectsAcceptedOwnerReceiptWithInconclusiveValidationEvidence(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), validationQualityFacts(t), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, managedValidationCandidate("validate-zero-work")); err != nil {
		t.Fatal(err)
	}
	executor := &qualityOperationExecutor{owner: controller, evidence: runtimecontract.RuntimeOperationEvidence{
		Summary: "command completed", Quality: common.EvidenceQualityInconclusive, ZeroWork: true,
		EvidenceReason: "no matching validation checks", EvidenceTags: []string{"zero_work"},
		Validation: &runtimecontract.RuntimeValidationObservation{OutputPreview: "no tests to run"},
	}}
	progressed, err := controller.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called != 1 || progressed.ActiveDispatch != nil || progressed.Decision.State != runtimecontract.ConvergenceStateNeedValidation ||
		progressed.Decision.Terminal != nil || progressed.Facts.Focus.ID != "focus-v2" {
		t.Fatalf("inconclusive validation did not return to same-focus validation repair: %+v calls=%d", progressed, executor.called)
	}
	if len(progressed.Facts.Evidence.Accepted) != 0 || len(progressed.Facts.Evidence.Rejected) != 1 || progressed.Facts.Failure == nil ||
		progressed.Facts.Failure.Class != runtimecontract.FailureClassCoverageAssessmentIncomplete ||
		progressed.Facts.Failure.Code != "validation_evidence_inconclusive" {
		t.Fatalf("inconclusive validation adjudication = evidence=%+v failure=%+v", progressed.Facts.Evidence, progressed.Facts.Failure)
	}
}

func TestRuntimeControllerRestartRejectsPersistedInconclusiveValidationReceiptWithoutReexecution(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), validationQualityFacts(t), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, managedValidationCandidate("validate-zero-work"))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", "validate-zero-work")
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, "validate-zero-work")
	if !ok {
		t.Fatal("claimed validation execution is missing")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: "validate-zero-work",
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: "command completed", Quality: common.EvidenceQualityInconclusive, ZeroWork: true,
			EvidenceReason: "no matching validation checks", Validation: &runtimecontract.RuntimeValidationObservation{OutputPreview: "no tests to run"},
		},
	}
	if _, _, err := controller.CommitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, receipt); err != nil {
		t.Fatal(err)
	}

	restarted, _ := NewRuntimeController(repository)
	executor := &failIfExecutedOperationExecutor{}
	progressed, err := restarted.ExecuteActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called || len(progressed.Facts.Evidence.Accepted) != 0 || len(progressed.Facts.Evidence.Rejected) != 1 ||
		progressed.Facts.Failure == nil || progressed.Facts.Failure.Code != "validation_evidence_inconclusive" {
		t.Fatalf("restart quality adjudication replayed or accepted weak evidence: %+v called=%v", progressed, executor.called)
	}
}

func TestRuntimeControllerExecutesManagedValidationWithSealedRecipeAfterResourceRefresh(t *testing.T) {
	aggregate := validationAggregate(t)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	if sealed.ActiveDispatch == nil || len(sealed.ActiveDispatch.ValidationRecipes) != 1 || sealed.ActiveDispatch.ValidationRecipes[0].ID != "test-managed-validation" {
		t.Fatalf("sealed recipe snapshot = %+v", sealed.ActiveDispatch)
	}

	// Simulate a resource refresh between seal and a fresh-process recovery. The
	// current Shell inventory now exposes only a different recipe; it must not
	// change a dispatch that was already authorized and claimed under the
	// previous owner snapshot.
	refreshedPrerequisite := runtimeV2TestManagedValidationPrerequisite("operation-admissibility-shell-refreshed")
	refreshedInventory := runtimeV2TestManagedValidationInventory(refreshedPrerequisite.ID)
	refreshedInventory.ValidationRecipes = refreshedInventory.ValidationRecipes[1:]
	refreshedInventory.ObservedRevision = "shell-policy-test-v2"
	refreshedInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(refreshedInventory)
	repository.aggregate.Facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{refreshedPrerequisite}
	repository.aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{refreshedInventory}

	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	executor := &sealedValidationRecipeExecutor{owner: restarted}
	if _, err := restarted.ExecuteActiveDispatch(context.Background(), "ws-1", "run-1", executor); err != nil {
		t.Fatal(err)
	}
	if len(executor.requests) != 1 || len(executor.requests[0].ValidationRecipes) != 1 {
		t.Fatalf("execution recipes = %+v", executor.requests)
	}
	got := executor.requests[0].ValidationRecipes[0]
	if got.ID != "test-managed-validation" || got.Command != "true" || len(got.Args) != 1 || got.Args[0] != "workorch-validation" {
		t.Fatalf("execution used refreshed inventory instead of sealed recipe: %+v", got)
	}
}

func beginAndAcceptCandidate(t *testing.T, controller *RuntimeController, aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	t.Helper()
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := RecordFixtureCandidateFeasibilities(context.Background(), controller, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate); err != nil {
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

func validationQualityFacts(t *testing.T) runtimecontract.RuntimeFacts {
	t.Helper()
	facts := runtimeV2FactsWithManagedValidationRecipe(controllerFacts())
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	return runtimeV2TestFactsWithCurrentFocusBindings(t, facts, controllerDefinition())
}

func managedValidationCandidate(operationID string) runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{
		Operation: runtimecontract.RuntimeOperation{
			ID: operationID, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeV2ManagedValidationParameters(),
		},
	}}}}
}
