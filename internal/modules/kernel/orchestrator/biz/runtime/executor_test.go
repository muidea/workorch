package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type failIfExecutedOperationExecutor struct {
	called bool
}

func runtimeTestOperationOwnerSource(kind string) (string, error) {
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
	if !found || descriptor.OwnerModuleID == "" {
		return "", fmt.Errorf("operation kind %q has no receipt owner", kind)
	}
	return descriptor.OwnerModuleID, nil
}

func TestRuntimeOperationReceiptJudgmentPreservesRejectedOwnerDiagnostic(t *testing.T) {
	operation := runtimecontract.RuntimeOperation{ID: "patch-2", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"main.go"}}
	execution := runtimecontract.RuntimeOperationExecutionRecord{OperationID: operation.ID, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable}
	originalFailure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "patch_replay_proof_mismatch",
		Signature: "patch-replay-proof-mismatch/patch-2", Message: "sealed before hash no longer matches", TargetRefs: []string{"main.go"},
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnosticFromFailureFact(runtimecontract.RuntimeModuleFile, originalFailure)
	diagnostic.OperationID = operation.ID
	receipt := runtimecontract.RuntimeOperationReceipt{
		OperationID: operation.ID, Outcome: runtimecontract.RuntimeOperationOutcomeRejected,
		ObservedEffectScope: runtimecontract.RuntimeEffectUnknown,
		Feedback: &runtimecontract.RuntimeFeedback{
			Domain: runtimecontract.RuntimeFeedbackDomainOperation, StableCategory: runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected),
			OriginalDiagnostic: diagnostic,
		},
	}
	result := RuntimeOperationExecutionResult{Status: ExecutionEvidenceStatusAccepted, Receipt: receipt}
	if err := adjudicateRuntimeOperationResult(runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}, execution, operation, &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionEvidenceStatusRejected || result.Failure == nil ||
		result.Failure.Class != runtimecontract.FailureClassExecutionFailure || result.Failure.Code != "patch_replay_proof_mismatch" ||
		result.Failure.OriginalDiagnostic == nil || result.Failure.OriginalDiagnostic.OriginalCode != "patch_replay_proof_mismatch" {
		t.Fatalf("rejected owner diagnostic was rewritten: %+v", result)
	}
}

func TestRuntimeOperationReceiptJudgmentStillRejectsAcceptedEffectMismatch(t *testing.T) {
	operation := runtimecontract.RuntimeOperation{ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"main.go"}}
	execution := runtimecontract.RuntimeOperationExecutionRecord{OperationID: operation.ID, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable}
	status, failure, err := runtimeOperationReceiptEvidenceJudgment(
		runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange},
		execution,
		operation,
		runtimecontract.RuntimeOperationReceipt{OperationID: operation.ID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone},
	)
	if err != nil || status != ExecutionEvidenceStatusRejected || failure == nil || failure.Class != runtimecontract.FailureClassEffectMismatch || failure.Code != "operation_effect_mismatch" {
		t.Fatalf("accepted effect mismatch judgment = status=%s failure=%+v err=%v", status, failure, err)
	}
}

func TestRuntimeValidationScopeTargetsOnlyUseSealedManagedValidationAuthority(t *testing.T) {
	managed := runtimecontract.RuntimeOperation{ID: "validate", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, TargetRefs: []string{"internal/lspbridge/validation_surface_test.go"}}
	dispatch := SealedDispatch{
		Candidate: runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: managed}}}},
		OperationTargetAuthorizations: []runtimecontract.RuntimeDispatchOperationTargetAuthorization{{
			OperationID: managed.ID, OperationHash: runtimecontract.StableRuntimeOperationHash(managed), RequireTargets: true,
			Target: runtimecontract.RuntimeOperationTargetAuthorization{
				Policy:               runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
				OwnerScopeTargetRefs: []string{"cmd/lsp-bridge/main.go", "internal/lspbridge/validation_surface_test.go"},
			},
		}},
	}
	got, err := runtimeValidationScopeTargetRefs(dispatch, managed)
	if err != nil || len(got) != 2 || got[0] != "cmd/lsp-bridge/main.go" || got[1] != "internal/lspbridge/validation_surface_test.go" {
		t.Fatalf("managed validation sealed target scope = %v err=%v", got, err)
	}
	got, err = runtimeValidationScopeTargetRefs(SealedDispatch{}, runtimecontract.RuntimeOperation{ID: "mutate", Kind: runtimecontract.StepKindShellCommand})
	if err != nil || len(got) != 0 {
		t.Fatalf("durable Shell command received validation target scope: %v err=%v", got, err)
	}

	selectorOnly := dispatch
	selectorOnly.OperationTargetAuthorizations = runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(dispatch.OperationTargetAuthorizations)
	selectorOnly.OperationTargetAuthorizations[0].Target.OwnerScopeTargetRefs = nil
	selectorOnly.OperationTargetAuthorizations[0].Target.OwnerSelectorRefs = []string{"workspace-validation-target"}
	got, err = runtimeValidationScopeTargetRefs(selectorOnly, managed)
	if err != nil || len(got) != 0 {
		t.Fatalf("selector-only managed validation invented a finite diagnostic scope: scope=%v err=%v", got, err)
	}
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
	ownerSource, err := runtimeTestOperationOwnerSource(request.Operation.Kind)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	_, committed, err := e.owner.commitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, ownerSource, receipt)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{
		EventID: "event-" + string(request.WorkspaceID) + "-" + string(request.RunID) + "-" + request.Operation.ID, OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
		Status: ExecutionEvidenceStatusAccepted, SourceRef: "runtime://" + request.Operation.ID,
		Evidence: receipt.Evidence, Receipt: committed,
	}, nil
}

type passingManagedValidationExecutor struct {
	owner *RuntimeController
}

func (e *passingManagedValidationExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	if len(request.ValidationRecipes) != 1 {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("managed validation fixture requires one sealed recipe, got %d", len(request.ValidationRecipes))
	}
	recipeID := request.ValidationRecipes[0].ID
	checkRef := "check/" + recipeID
	evidence := runtimecontract.RuntimeOperationEvidence{
		Summary: "current workspace state validated",
		Validation: &runtimecontract.RuntimeValidationObservation{
			RecipeID: recipeID, ExecutedChecks: []string{checkRef}, PassedChecks: []string{checkRef},
		},
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...), Evidence: evidence,
	}
	ownerSource, err := runtimeTestOperationOwnerSource(request.Operation.Kind)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	_, committed, err := e.owner.commitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, ownerSource, receipt)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	return RuntimeOperationExecutionResult{
		EventID: "event-current-state-validation", OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind,
		Status: ExecutionEvidenceStatusAccepted, SourceRef: "runtime://current-state-validation", Evidence: evidence, Receipt: committed,
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
	ownerSource, err := runtimeTestOperationOwnerSource(request.Operation.Kind)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	_, committed, err := e.owner.commitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, ownerSource, receipt)
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
	ownerSource, err := runtimeTestOperationOwnerSource(request.Operation.Kind)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	_, committed, err := e.owner.commitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, ownerSource, receipt)
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
	ownerSource, err := runtimeTestOperationOwnerSource(request.Operation.Kind)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	_, committed, err := e.owner.commitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, ownerSource, receipt)
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
	ownerSource, err := runtimeTestOperationOwnerSource(request.Operation.Kind)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	_, committed, err := e.owner.commitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, ownerSource, receipt)
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
	ownerSource, err := runtimeTestOperationOwnerSource(request.Operation.Kind)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	if _, _, err := e.owner.commitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, ownerSource, receipt); err != nil {
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
		descriptor, _ := runtimecontract.RuntimeOperationDescriptorForKind(request.Operation.Kind)
		feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
			WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID, DispatchID: request.DispatchID,
		}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(receipt.Outcome), descriptor.OwnerModuleID, *result.Failure)
		if err != nil {
			return RuntimeOperationExecutionResult{}, err
		}
		feedback.OriginalDiagnostic.OperationID = request.Operation.ID
		receipt.Feedback = &feedback
	}
	if e.owner != nil {
		ownerSource, err := runtimeTestOperationOwnerSource(request.Operation.Kind)
		if err != nil {
			return RuntimeOperationExecutionResult{}, err
		}
		_, committed, err := e.owner.commitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, ownerSource, receipt)
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
	facts := multiOperationDependencyFixtureFacts(t)
	facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: runtimecontract.StepKindFileEdit}}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	candidate := multiOperationCandidate()
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	firstOperationID := runtimeTestActiveOperationIDForTarget(t, sealed, "a.txt")
	secondOperationID := runtimeTestActiveOperationIDForTarget(t, sealed, "b.txt")
	executor := &recordingOperationExecutor{owner: controller}
	completed, err := controller.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatalf("executeActiveDispatch: %v", err)
	}
	if len(executor.requests) != 2 || executor.requests[0].Operation.ID != firstOperationID || executor.requests[1].Operation.ID != secondOperationID ||
		executor.requests[0].Operation.TargetRefs[0] != "a.txt" || executor.requests[1].Operation.TargetRefs[0] != "b.txt" {
		t.Fatalf("execution order = %+v", executor.requests)
	}
	if len(executor.requests[0].EvidencePolicies) != 1 || executor.requests[0].EvidencePolicies[0].ID != "policy-1" {
		t.Fatalf("executor request lost typed evidence policy: %+v", executor.requests[0].EvidencePolicies)
	}
	if completed.ActiveDispatch != nil || len(completed.CompletedDispatches) != 1 || completed.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("completed aggregate = %+v", completed)
	}
	if completed.EvidenceSequence != 2 || len(completed.Facts.Evidence.Accepted) != 2 {
		t.Fatalf("evidence state = sequence %d facts %+v", completed.EvidenceSequence, completed.Facts.Evidence)
	}
}

func TestTerminatedRuntimeCannotClaimOrExecuteSealedDispatch(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := multiOperationDependencyFixtureFacts(t)
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, multiOperationCandidate())
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ActiveDispatch == nil {
		t.Fatal("candidate did not seal an active dispatch")
	}
	terminated, err := controller.requestTermination(ctx, facts.WorkspaceID, facts.RunID, runtimecontract.RuntimeTerminationSuperseded, "run-owner", "replacement run activated")
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := terminated.StateRevision
	beforeSaves := repository.saves
	executor := &failIfExecutedOperationExecutor{}

	if _, err := controller.executeActiveDispatch(ctx, facts.WorkspaceID, facts.RunID, executor); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("execute after termination err = %v, want conflict", err)
	}
	if executor.called {
		t.Fatal("terminated runtime called capability owner")
	}
	if repository.aggregate.StateRevision != beforeRevision || repository.saves != beforeSaves || len(repository.aggregate.OperationReceipts) != 0 || len(repository.aggregate.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("terminated dispatch progressed: revision=%d saves=%d receipts=%d evidence=%d", repository.aggregate.StateRevision, repository.saves, len(repository.aggregate.OperationReceipts), len(repository.aggregate.Facts.Evidence.Accepted))
	}
	for _, execution := range repository.aggregate.ActiveDispatch.OperationExecutions {
		if execution.State != runtimecontract.OperationExecutionSealed || execution.Fence != 0 {
			t.Fatalf("terminated dispatch execution changed: %+v", execution)
		}
	}
}

func TestRuntimeControllerRestartContinuesSealedDispatchAfterResourceChangingReceipt(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus.TargetRefs = []string{"main.go", "other.go"}
		catalog := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: "test-semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write",
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
			}},
			TargetRefs: []string{"main.go", "other.go"}, ObservedRevision: "tree-v1",
		}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	})
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
			return operation
		}()},
	}}}
	sealed := sealCandidateForTest(t, repository, candidate)
	firstOperationID := runtimeTestActiveOperationIDForTarget(t, sealed, "main.go")
	secondOperationID := runtimeTestActiveOperationIDForTarget(t, sealed, "other.go")
	claimed, err := controller.claimOperationExecution(ctx, "ws-1", "run-1", firstOperationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, firstOperationID)
	if !ok {
		t.Fatal("first operation execution was not claimed")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: firstOperationID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
		TargetRefs: []string{"main.go"},
		Evidence:   runtimecontract.RuntimeOperationEvidence{Summary: "first durable effect"},
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2",
		}},
	}
	if _, _, err := controller.commitOperationReceipt(ctx, "ws-1", "run-1", sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt); err != nil {
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
	completed, err := restarted.executeActiveDispatch(ctx, "ws-1", "run-1", executor)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.requests) != 1 || executor.requests[0].Operation.ID != secondOperationID ||
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
	// This test stops a rejected operation before it can close coverage.
	facts := multiOperationDependencyFixtureFacts(t)
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, multiOperationCandidate())
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingOperationExecutor{rejectID: runtimeTestActiveOperationIDForTarget(t, sealed, "a.txt"), owner: controller}
	failed, err := controller.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatalf("executeActiveDispatch: %v", err)
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
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("recovered")},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", operationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !ok {
		t.Fatal("claimed execution is missing")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: operationID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
		TargetRefs: []string{"target.txt"},
		Evidence:   runtimecontract.RuntimeOperationEvidence{Summary: "owner committed before crash", EvidenceRefs: []string{"file://target.txt"}},
	}
	if _, _, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt); err != nil {
		t.Fatal(err)
	}

	restarted, _ := NewRuntimeController(repository)
	executor := &failIfExecutedOperationExecutor{}
	completed, err := restarted.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called {
		t.Fatal("owner side effect was replayed after persisted receipt")
	}
	if len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 || completed.EvidenceSequence != 1 {
		t.Fatalf("receipt/evidence recovery is not exactly once: receipts=%d evidence=%d sequence=%d", len(completed.OperationReceipts), len(completed.Facts.Evidence.Accepted), completed.EvidenceSequence)
	}

	replayed, err := restarted.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
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
	// Response-loss recovery is an SM-3 receipt fence test. Keep the fixture on
	// an explicitly sealed non-assessment focus so this test does not conflate
	// it with the separate SM-2 assessment transition of semantic planning.
	facts := controllerFacts()
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("recovered")},
	}}}}); err != nil {
		t.Fatal(err)
	}

	completed, err := controller.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", &receiptCommittedResponseLostExecutor{owner: controller})
	if err != nil {
		t.Fatalf("response-loss recovery: %v", err)
	}
	if completed.ActiveDispatch != nil || completed.Decision.State != runtimecontract.ConvergenceStateNeedAssessment ||
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
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("unknown")},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	if _, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", operationID); err != nil {
		t.Fatal(err)
	}
	executor := &failIfExecutedOperationExecutor{}
	_, err = controller.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	var outcomeUnknown *OperationOutcomeUnknownError
	if !errors.As(err, &outcomeUnknown) {
		t.Fatalf("claimed recovery err = %v, want outcome_unknown", err)
	}
	if executor.called {
		t.Fatal("claimed operation without receipt was blindly replayed")
	}
	loaded, _ := controller.Load(context.Background(), "ws-v2", "run-v2")
	execution, ok := operationExecutionRecord(loaded.ActiveDispatch.OperationExecutions, operationID)
	if !ok || execution.State != runtimecontract.OperationExecutionOutcomeUnknown || len(loaded.OperationReceipts) != 0 || len(loaded.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("unknown recovery state = %+v aggregate=%+v", execution, loaded)
	}
}

func TestMissingOperationExecutionLedgerFailsWithoutSaveOrBudgetMutation(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("content")},
	}}}})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal aggregate=%+v err=%v", sealed, err)
	}
	repository.aggregate.ActiveDispatch.OperationExecutions = nil
	saves := repository.saves
	budget := repository.aggregate.ExecutionBudget.Clone()
	for name, invoke := range map[string]func() error{
		"claim": func() error {
			_, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", "write-1")
			return err
		},
		"outcome_unknown": func() error {
			_, err := controller.candidateDispatch.markOperationExecutionOutcomeUnknown(context.Background(), "ws-v2", "run-v2", "write-1")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := invoke(); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("missing execution ledger err=%v, want conflict", err)
			}
			if repository.saves != saves || len(repository.aggregate.ActiveDispatch.OperationExecutions) != 0 ||
				!reflect.DeepEqual(repository.aggregate.ExecutionBudget, budget) {
				t.Fatalf("missing execution ledger mutated persistence: saves=%d aggregate=%+v", repository.saves, repository.aggregate)
			}
		})
	}
}

func TestRuntimeOperationReplayAuthorityUsesOnlySealedIdempotency(t *testing.T) {
	execution := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: "operation-1", ExecutionKey: "execution-1",
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		IdempotencyClass:    runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		State:               runtimecontract.OperationExecutionClaimed,
		Fence:               1,
		ClaimedAt:           runtimecontract.RuntimeOperationClaimMarkerInitial,
	}
	if runtimeOperationHasReplayAuthority(execution) {
		t.Fatal("at-most-once sealed execution gained replay authority")
	}
	execution.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencySafeToReplay
	if !runtimeOperationHasReplayAuthority(execution) {
		t.Fatal("safe-to-replay sealed execution lost replay authority")
	}
	execution.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencyOwnerManaged
	if !runtimeOperationHasReplayAuthority(execution) {
		t.Fatal("owner-managed sealed execution lost replay authority")
	}
	execution.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencyAtMostOnce
	execution.AdmissibilitySnapshotID = "catalog-1"
	execution.ReplayProof = &runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationID: execution.OperationID,
		TargetRefs: []string{"target.txt"}, BeforeHash: "before", AfterHash: "after",
		AdmissibilitySnapshotID: "catalog-1",
	}
	if !runtimeOperationHasReplayAuthority(execution) {
		t.Fatal("exact sealed replay proof did not authorize recovery")
	}
}

func TestRuntimeControllerDoesNotReplayClaimedManagedValidationWithoutReplayProof(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := runtimeFactsWithManagedValidationRecipe(controllerFacts())
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}}
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	facts = aggregate.Facts
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, managedValidationCandidate("read-1"))
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	if _, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", operationID); err != nil {
		t.Fatal(err)
	}
	restarted, _ := NewRuntimeController(repository)
	executor := &failIfExecutedOperationExecutor{}
	_, err = restarted.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	var outcomeUnknown *OperationOutcomeUnknownError
	if !errors.As(err, &outcomeUnknown) {
		t.Fatalf("managed validation recovery err = %v, want outcome_unknown", err)
	}
	if executor.called {
		t.Fatal("claimed managed validation was blindly replayed")
	}
	loaded, loadErr := restarted.Load(context.Background(), "ws-v2", "run-v2")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	execution, ok := operationExecutionRecord(loaded.ActiveDispatch.OperationExecutions, operationID)
	if !ok || execution.State != runtimecontract.OperationExecutionOutcomeUnknown ||
		loaded.Facts.Recovery.OperationRecoveryAttempts != 0 || len(loaded.OperationReceipts) != 0 || len(loaded.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("managed validation recovery sealed=%+v loaded=%+v execution=%+v", sealed, loaded, execution)
	}
}

func TestRuntimeControllerRecoversClaimedAtMostOnceOperationOnlyWithSealedReplayProof(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Capabilities = runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{runtimecontract.StepKindFileWrite}}
	// The semantic test compiler supplies the exact File catalog after the
	// unbound activation is persisted. Keeping a second handwritten File
	// snapshot here would create two owner identities for one feasibility.
	facts.Resources.OperationAdmissibilities = nil
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	facts = aggregate.Facts
	target, content := "target.txt", "after\n"
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
		Parameters: runtimeFileWriteParameters(content),
	}}}}}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AdmitCandidateProposalForTest(context.Background(), "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	loaded, err := controller.Load(context.Background(), "ws-v2", "run-v2")
	if err != nil || len(loaded.Facts.Resources.OperationAdmissibilities) != 1 {
		t.Fatalf("load sealed File catalog: aggregate=%+v err=%v", loaded, err)
	}
	operationID := runtimeTestCandidateOperationID(t, *loaded.CandidateAttempts[len(loaded.CandidateAttempts)-1].Proposal, 0)
	catalog := loaded.Facts.Resources.OperationAdmissibilities[0].Normalized()
	proof := runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationID: operationID, TargetRefs: []string{target},
		BeforeHash: "before", AfterHash: "after", AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
	}
	if _, err := controller.recordCandidateFeasibility(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: operationID, AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
		CandidateManifestHash: runtimeTestPendingCandidateManifestHash(t, controller, "ws-v2", "run-v2", 0), Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible}, ReplayProof: &proof,
	}); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AdmitAndSealCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("seal replay-proof candidate aggregate=%+v err=%v", sealed.Aggregate, err)
	}
	execution, ok := operationExecutionRecord(sealed.Aggregate.ActiveDispatch.OperationExecutions, operationID)
	if !ok || execution.ReplayProof == nil || execution.ReplayProof.AfterHash != "after" {
		t.Fatalf("sealed execution omitted replay proof: %+v", execution)
	}
	executor := &replayProofRecoveryExecutor{owner: controller}
	completed, err := controller.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.requests) != 2 || executor.requests[0].Execution.ExecutionKey != executor.requests[1].Execution.ExecutionKey || executor.requests[0].Execution.Fence != executor.requests[1].Execution.Fence || executor.requests[1].Execution.ReplayProof == nil {
		t.Fatalf("recovery did not reuse sealed execution identity/proof: %+v", executor.requests)
	}
	if completed.ActiveDispatch != nil || completed.Decision.State != runtimecontract.ConvergenceStateNeedAssessment || completed.Facts.Recovery.OperationRecoveryAttempts != 1 || len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("replay-proof recovery did not converge exactly once: %+v", completed)
	}
}

func TestRuntimeControllerRoutesStateChangeNoOpToOneBoundedCurrentStateValidation(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := runtimeFactsWithManagedValidationRecipe(controllerFacts())
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	ownerCatalogs := facts.Resources.OperationAdmissibilities[:0]
	for _, catalog := range facts.Resources.OperationAdmissibilities {
		if catalog.Normalized().OwnerModuleID != runtimecontract.RuntimeModuleFile {
			ownerCatalogs = append(ownerCatalogs, catalog)
		}
	}
	facts.Resources.OperationAdmissibilities = ownerCatalogs
	// This isolates the policy that follows a no-op durable operation. A
	// semantic focus requires assessment after a valid state change, which is
	// outside this bounded validation-path assertion.
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	facts = aggregate.Facts
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("same")},
	}}}}); err != nil {
		t.Fatal(err)
	}
	progressed, err := controller.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", &noOpOperationExecutor{owner: controller})
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
	if _, err := beginAndAcceptCandidate(t, controller, progressed, managedValidationCandidate("inspect-current-state")); err != nil {
		t.Fatal(err)
	}
	inspected, err := controller.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", &passingManagedValidationExecutor{owner: controller})
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Decision.State != runtimecontract.ConvergenceStateNeedAssessment || inspected.Decision.Terminal != nil {
		t.Fatalf("validated no-op state did not advance to canonical assessment: decision=%+v", inspected.Decision)
	}
	if len(inspected.Facts.Evidence.Accepted) != 1 || inspected.Facts.Evidence.Accepted[0].Kind != runtimecontract.EvidenceKindValidation || len(inspected.Facts.Evidence.Rejected) != 1 {
		t.Fatalf("no-op path fabricated state-change evidence: accepted=%+v rejected=%+v", inspected.Facts.Evidence.Accepted, inspected.Facts.Evidence.Rejected)
	}
}

func TestRuntimeControllerRestartAtomicallyInvalidatesUncommittableOwnerReceipt(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("content")},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", operationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !ok {
		t.Fatal("claimed execution is missing")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner observed no durable change"},
	}
	if _, _, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt); err != nil {
		t.Fatal(err)
	}

	restarted, _ := NewRuntimeController(repository)
	executor := &failIfExecutedOperationExecutor{}
	progressed, err := restarted.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
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

func TestRuntimeControllerRestartRejectsPersistedMixedEffectDispatchWithoutReplay(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("content")},
	}}}})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal aggregate=%+v err=%v", sealed, err)
	}
	// Inject an obsolete mixed-effect shape. Current Runtime does not migrate
	// persisted authority: restart must fail closed before any owner call.
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
	if _, err := restarted.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor); err == nil {
		t.Fatal("obsolete mixed-effect dispatch was accepted during restart")
	}
	if executor.called || repository.aggregate.ActiveDispatch == nil || len(repository.aggregate.CompletedDispatches) != 0 {
		t.Fatalf("invalid persisted dispatch caused an owner call or migration: aggregate=%+v called=%v", repository.aggregate, executor.called)
	}
}

func TestRecordFailureFactRejectsActiveDispatch(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("content")},
	}}}})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal aggregate=%+v err=%v", sealed, err)
	}
	revision := repository.aggregate.StateRevision
	_, err = controller.recordFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeInvariantDefect, Code: "generic", Signature: "generic", Message: "generic failure",
	})
	if !errors.Is(err, common.ErrConflict) || repository.aggregate.StateRevision != revision || repository.aggregate.ActiveDispatch == nil {
		t.Fatalf("generic failure changed an active dispatch: aggregate=%+v err=%v", repository.aggregate, err)
	}
}

func TestRecordDriveFailureFactAtomicallyInvalidatesActiveDispatch(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("content")},
	}}}})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal aggregate=%+v err=%v", sealed, err)
	}

	failed, err := controller.recordDriveFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed",
		Signature: "runtime-drive-failed", Message: "typed drive contract failure",
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
	replayed, err := controller.recordDriveFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Message: "duplicate drive failure",
	})
	if err != nil || repository.saves != saves || replayed.StateRevision != failed.StateRevision {
		t.Fatalf("duplicate terminal drive failure changed state: aggregate=%+v saves=%d want=%d err=%v", replayed, repository.saves, saves, err)
	}
}

func TestRuntimeControllerRejectsAcceptedOwnerReceiptWithInconclusiveValidationEvidence(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := validationQualityFacts(t)
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	if _, err := beginAndAcceptCandidate(t, controller, aggregate, managedValidationCandidate("validate-zero-work")); err != nil {
		t.Fatal(err)
	}
	executor := &qualityOperationExecutor{owner: controller, evidence: runtimecontract.RuntimeOperationEvidence{
		Summary: "command completed", Quality: common.EvidenceQualityInconclusive, ZeroWork: true,
		EvidenceReason: "no matching validation checks", EvidenceTags: []string{"zero_work"},
		Validation: &runtimecontract.RuntimeValidationObservation{OutputPreview: "no tests to run"},
	}}
	progressed, err := controller.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called != 1 || progressed.ActiveDispatch != nil || progressed.Decision.State != runtimecontract.ConvergenceStateNeedValidation ||
		progressed.Decision.Terminal != nil || progressed.Facts.Focus.ID != aggregate.Facts.Focus.ID {
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
	facts := validationQualityFacts(t)
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, managedValidationCandidate("validate-zero-work"))
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", operationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !ok {
		t.Fatal("claimed validation execution is missing")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: operationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: "command completed", Quality: common.EvidenceQualityInconclusive, ZeroWork: true,
			EvidenceReason: "no matching validation checks", Validation: &runtimecontract.RuntimeValidationObservation{OutputPreview: "no tests to run"},
		},
	}
	if _, _, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleShell, receipt); err != nil {
		t.Fatal(err)
	}

	restarted, _ := NewRuntimeController(repository)
	executor := &failIfExecutedOperationExecutor{}
	progressed, err := restarted.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
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
	operation := sealed.ActiveDispatch.Candidate.Validation.Operations[0].Operation
	sealedValidationScope, err := runtimeValidationScopeTargetRefs(*sealed.ActiveDispatch, operation)
	if err != nil {
		t.Fatalf("sealed validation scope: %v", err)
	}

	// Simulate a resource refresh between seal and a fresh-process recovery. The
	// current Shell inventory now exposes only a different recipe; it must not
	// change a dispatch that was already authorized and claimed under the
	// previous owner snapshot.
	refreshedPrerequisite := runtimeTestManagedValidationPrerequisite("operation-admissibility-shell-refreshed")
	refreshedInventory := runtimeTestManagedValidationInventory(refreshedPrerequisite.ID)
	refreshedInventory.ValidationRecipes = refreshedInventory.ValidationRecipes[1:]
	refreshedInventory.ValidationSurfaces = refreshedInventory.ValidationSurfaces[1:]
	refreshedInventory.ObservedRevision = "shell-policy-test-v2"
	refreshedInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(refreshedInventory)
	refreshedObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: refreshedPrerequisite.ID, Kind: refreshedPrerequisite.Kind,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, ContentHash: refreshedInventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell,
			SubjectRef: runtimecontract.RuntimeModuleShell, Revision: refreshedInventory.ObservedRevision,
		}},
	}
	refreshedObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(refreshedObservation)
	repository.aggregate.Facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{refreshedPrerequisite}
	repository.aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{refreshedInventory}
	repository.aggregate.Facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{refreshedObservation}
	repository.aggregate.Facts.Resources.Manifest = &runtimecontract.WorkspaceManifest{
		Revision: "tree-after-seal", TargetRefs: []string{"mutable-current-only.go"},
	}

	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	executor := &sealedValidationRecipeExecutor{owner: restarted}
	if _, err := restarted.executeActiveDispatch(context.Background(), "ws-1", "run-1", executor); err != nil {
		t.Fatal(err)
	}
	if len(executor.requests) != 1 || len(executor.requests[0].ValidationRecipes) != 1 {
		t.Fatalf("execution recipes = %+v", executor.requests)
	}
	if !reflect.DeepEqual(executor.requests[0].ValidationScopeTargetRefs, sealedValidationScope) {
		t.Fatalf("execution rebuilt validation scope after restart: got=%v sealed=%v", executor.requests[0].ValidationScopeTargetRefs, sealedValidationScope)
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
		{Operation: runtimecontract.RuntimeOperation{ID: "write-b", Kind: "file/write", RequiredOperationRef: "write-b", DependsOn: []string{"write-a"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"b.txt"}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &contentB}}}},
		{Operation: runtimecontract.RuntimeOperation{ID: "write-a", Kind: "file/write", RequiredOperationRef: "write-a", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"a.txt"}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &contentA}}}},
	}}}
}

// multiOperationFixtureFacts gives the File owner an explicit bounded catalog
// for both fixture targets. A selector on the proof binding does not grant
// authority outside the owner catalog, so these tests must not inherit the
// generic target.txt inventory.
func multiOperationFixtureFacts(t *testing.T) runtimecontract.RuntimeFacts {
	t.Helper()
	facts := controllerFacts()
	facts.Focus.TargetRefs = []string{"a.txt", "b.txt"}
	if len(facts.Resources.OperationAdmissibilities) != 1 {
		t.Fatalf("multi-operation fixture requires one File catalog: %+v", facts.Resources.OperationAdmissibilities)
	}
	catalog := facts.Resources.OperationAdmissibilities[0].Normalized()
	catalog.TargetRefs = []string{"a.txt", "b.txt"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	return facts
}

func multiOperationDependencyFixtureFacts(t *testing.T) runtimecontract.RuntimeFacts {
	t.Helper()
	facts := multiOperationFixtureFacts(t)
	sequence := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: "file-sequence", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		CapabilityStateKind: "test_operation_sequence", SubjectRef: "workspace_deliverable",
		ObservedRevision: "file-sequence-v1", ContentHash: "file-sequence-content-v1",
		AcquisitionKey: "file-sequence-acquisition-v1", QueryReceiptRef: "file-sequence-receipt-v1",
		Disposition: runtimecontract.RuntimeCapabilityStateSequenceRequired,
		RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
			{RequirementID: "write-a", Kind: runtimecontract.StepKindFileWrite, TargetRef: "a.txt"},
			{RequirementID: "write-b", Kind: runtimecontract.StepKindFileWrite, TargetRef: "b.txt", DependsOnRequirementIDs: []string{"write-a"}},
		},
	}
	sequence.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(sequence)
	facts.Resources.CapabilityStates = append(facts.Resources.CapabilityStates, sequence)
	return facts
}

func runtimeTestActiveOperationIDForTarget(t *testing.T, aggregate RuntimeAggregate, target string) string {
	t.Helper()
	if aggregate.ActiveDispatch == nil {
		t.Fatal("fixture has no active dispatch")
	}
	for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
		if reflect.DeepEqual(operation.TargetRefs, []string{target}) {
			return operation.ID
		}
	}
	t.Fatalf("fixture has no active operation for target %q: %+v", target, aggregate.ActiveDispatch.Candidate)
	return ""
}

func validationQualityFacts(t *testing.T) runtimecontract.RuntimeFacts {
	t.Helper()
	facts := runtimeFactsWithManagedValidationRecipe(controllerFacts())
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	return facts
}

func managedValidationCandidate(operationID string) runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{
		Operation: runtimecontract.RuntimeOperation{
			ID: operationID, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeManagedValidationParameters(),
		},
	}}}}
}
