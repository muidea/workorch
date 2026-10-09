package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type controllerRepository struct {
	aggregate *RuntimeAggregate
	saves     int
}

// scopedControllerRepository is a test-only, concurrency-safe repository for
// proving that one controller never lets a repair transition cross the
// workspace/run persistence boundary.
type scopedControllerRepository struct {
	mu         sync.Mutex
	aggregates map[string]RuntimeAggregate
}

func runtimeAggregateScopeKey(workspaceID common.WorkspaceID, runID common.RunID) string {
	return string(workspaceID) + "\x00" + string(runID)
}

func (r *scopedControllerRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if r == nil {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	aggregate, found := r.aggregates[runtimeAggregateScopeKey(workspaceID, runID)]
	if !found {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return aggregate.CloneRaw(), nil
}

func (r *scopedControllerRepository) Save(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := runtimeAggregateScopeKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	current, found := r.aggregates[key]
	if found && current.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	if !found && expectedRevision != 0 {
		return common.ErrConflict
	}
	r.aggregates[key] = aggregate.CloneRaw()
	return nil
}

func attachRuntimeTestScopedOwnerReceipt(t *testing.T, repository *scopedControllerRepository, evidence *ExecutionEvidence) {
	t.Helper()
	if err := attachRuntimeTestScopedOwnerReceiptResult(repository, evidence); err != nil {
		t.Fatal(err)
	}
}

func attachRuntimeTestScopedOwnerReceiptResult(repository *scopedControllerRepository, evidence *ExecutionEvidence) error {
	if repository == nil || evidence == nil {
		return fmt.Errorf("scoped owner receipt fixture requires repository and evidence")
	}
	key := runtimeAggregateScopeKey(evidence.WorkspaceID, evidence.RunID)
	repository.mu.Lock()
	aggregate, found := repository.aggregates[key]
	repository.mu.Unlock()
	if !found {
		return fmt.Errorf("scoped owner receipt aggregate %q is missing", key)
	}
	if aggregate.ActiveDispatch == nil || evidence.Status != ExecutionEvidenceStatusAccepted {
		return fmt.Errorf("scoped owner receipt fixture requires an active accepted execution")
	}
	var execution *runtimecontract.RuntimeOperationExecutionRecord
	for index := range aggregate.ActiveDispatch.OperationExecutions {
		if aggregate.ActiveDispatch.OperationExecutions[index].OperationID == evidence.OperationID {
			execution = &aggregate.ActiveDispatch.OperationExecutions[index]
			break
		}
	}
	if execution == nil {
		return fmt.Errorf("scoped owner receipt operation %q has no sealed execution: %+v", evidence.OperationID, aggregate.ActiveDispatch.OperationExecutions)
	}
	operation, found := candidateOperationByID(aggregate.ActiveDispatch.Candidate, evidence.OperationID)
	if !found {
		return fmt.Errorf("scoped owner receipt operation %q is missing from sealed candidate", evidence.OperationID)
	}
	if execution.Fence == 0 {
		execution.Fence = 1
	}
	outcome := runtimecontract.RuntimeOperationOutcomeAccepted
	observedEffectScope := execution.ExpectedEffectScope
	if evidence.NoOp {
		outcome = runtimecontract.RuntimeOperationOutcomeNoOp
		observedEffectScope = runtimecontract.RuntimeEffectNone
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID,
		Outcome: outcome, ObservedEffectScope: observedEffectScope,
		Changed: !evidence.NoOp && runtimecontract.RuntimeEffectIsDurable(execution.ExpectedEffectScope), TargetRefs: append([]string(nil), operation.TargetRefs...), Evidence: evidence.Evidence,
	}.Normalized()
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	execution.State = runtimecontract.OperationExecutionOwnerCommitted
	if execution.ClaimedAt == "" {
		execution.ClaimedAt = runtimecontract.RuntimeOperationClaimMarkerInitial
	}
	execution.OwnerReceiptRef = "runtime-receipt/" + execution.ExecutionKey
	execution.OwnerReceiptHash = receiptHash
	aggregate.OperationReceipts = append(aggregate.OperationReceipts, receipt)
	evidence.Receipt = &receipt
	evidence.ObservedEffectScope = receipt.ObservedEffectScope
	evidence.OwnerReceiptRef = execution.OwnerReceiptRef
	evidence.OwnerReceiptHash = receiptHash
	repository.mu.Lock()
	current, stillFound := repository.aggregates[key]
	if !stillFound || current.StateRevision != aggregate.StateRevision {
		repository.mu.Unlock()
		return fmt.Errorf("scoped owner receipt aggregate %q changed during owner commit", key)
	}
	repository.aggregates[key] = aggregate
	repository.mu.Unlock()
	return nil
}

func (r *controllerRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if r.aggregate == nil {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	if r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return r.aggregate.CloneRaw(), nil
}

func (r *controllerRepository) Save(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	currentRevision := uint64(0)
	if r.aggregate != nil {
		currentRevision = r.aggregate.StateRevision
	}
	if currentRevision != expectedRevision {
		return common.ErrConflict
	}
	value := aggregate.CloneRaw()
	r.aggregate = &value
	r.saves++
	return nil
}

func TestRuntimeControllerInitializeRunsPreflightReduceAndPersistOnce(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	requirements := []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindManifest, Reason: "manifest required"}}
	initialized, err := controller.initialize(context.Background(), facts, controllerDefinition(), requirements)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if initialized.StateRevision != 1 || initialized.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("initialized aggregate = %+v", initialized)
	}
	if initialized.ResultFeedbackVersion != runtimecontract.RuntimeResultFeedbackVersionCurrent {
		t.Fatalf("new Run did not seal the current feedback renderer: %d", initialized.ResultFeedbackVersion)
	}
	if got, want := initialized.Facts.Activation.ExecutionContract.OperationContractRevision, runtimecontract.RuntimeOperationContractRevision(); got != want {
		t.Fatalf("operation contract revision = %q, want %q", got, want)
	}
	if len(initialized.Facts.Prerequisites.Pending) != 1 || repository.saves != 1 {
		t.Fatalf("pending=%+v saves=%d", initialized.Facts.Prerequisites.Pending, repository.saves)
	}

	restarted, err := controller.initialize(context.Background(), facts, controllerDefinition(), requirements)
	if err != nil {
		t.Fatalf("restart initialize: %v", err)
	}
	if repository.saves != 1 {
		t.Fatalf("restart performed another save: %d", repository.saves)
	}
	if restarted.Decision.AuthoritySnapshot.ID != initialized.Decision.AuthoritySnapshot.ID || restarted.Decision.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("restart changed authority: before=%+v after=%+v", initialized.Decision.AuthoritySnapshot, restarted.Decision.AuthoritySnapshot)
	}
	if restarted.ResultFeedbackVersion != initialized.ResultFeedbackVersion {
		t.Fatal("restart changed the sealed feedback renderer")
	}
}

func TestRuntimeControllerInitializeRejectsNoncurrentFeedbackVersionWithoutWrite(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts, definition := controllerFacts(), controllerDefinition()
	if _, err := controller.initialize(context.Background(), facts, definition, nil); err != nil {
		t.Fatal(err)
	}
	// Unsupported archives fail closed without upgrading or writing them.
	repository.aggregate.ResultFeedbackVersion = 0
	before, err := json.Marshal(repository.aggregate)
	if err != nil {
		t.Fatal(err)
	}
	restartedController, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, err = restartedController.initialize(context.Background(), facts, definition, nil)
	if err == nil {
		t.Fatal("unsupported feedback version resumed")
	}
	after, err := json.Marshal(repository.aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != 1 || string(before) != string(after) {
		t.Fatal("restart rewrote legacy aggregate or feedback renderer")
	}
}

func TestRuntimeControllerTerminationRetryAcceptsPersistedIdentityAfterTerminalProjection(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminated, err := controller.requestTermination(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, " superseded ", " blocks/run ", " replacement ")
	if err != nil {
		t.Fatal(err)
	}
	if terminated.Termination == nil || terminated.Termination.Kind != runtimecontract.RuntimeTerminationSuperseded || terminated.Termination.RequestedBy != "blocks/run" || terminated.Termination.Reason != "replacement" {
		t.Fatalf("termination ingress was not canonicalized: %+v", terminated.Termination)
	}
	saves := repository.saves
	// Model response loss after the persisted administrative termination CAS. A
	// retry carries the same durable identity and must remain a zero-write read.
	retried, err := controller.requestTermination(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeTerminationSuperseded, "blocks/run", "replacement")
	if err != nil {
		t.Fatalf("retry persisted termination: %v", err)
	}
	if retried.Termination == nil || retried.Termination.Kind != terminated.Termination.Kind || repository.saves != saves {
		t.Fatalf("retry aggregate=%+v saves=%d, want unchanged saves=%d", retried.Termination, repository.saves, saves)
	}
	if _, err := controller.requestTermination(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeTerminationCancelled, "blocks/run", "different"); err == nil {
		t.Fatal("different termination kind was accepted after terminal projection")
	}
	// A corrupt fresh-load carrying both terminal authorities is not an
	// idempotent retry of either one.
	repository.aggregate.Decision.State = runtimecontract.ConvergenceStateComplete
	repository.aggregate.Decision.Terminal = &runtimecontract.RuntimeTerminalDecision{Code: "complete", Summary: "complete"}
	if _, err := controller.requestTermination(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimecontract.RuntimeTerminationSuperseded, "blocks/run", "replacement"); err == nil {
		t.Fatal("dual convergence and administrative terminal was accepted as a duplicate")
	}
}

func TestRuntimeControllerActivationInternalEventIsTheBootstrapMutationBoundary(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	definition := controllerDefinition()

	committed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventActivationObserved, Scope: RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID,
			RunID: facts.RunID}, Activation: &RuntimeInternalActivationPayload{ActivationFacts: &facts,
			ActivationDefinition: &definition},
	})
	if err != nil {
		t.Fatalf("activation internal event: %v", err)
	}
	if committed.Aggregate.StateRevision != 1 || repository.saves != 1 {
		t.Fatalf("activation aggregate=%+v saves=%d, want one bootstrap commit", committed.Aggregate, repository.saves)
	}

	wrongScope := facts
	wrongScope.RunID = "run-other"
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventActivationObserved, Scope: RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID,
			RunID: facts.RunID}, Activation: &RuntimeInternalActivationPayload{ActivationFacts: &wrongScope,
			ActivationDefinition: &definition},
	}); err == nil {
		t.Fatal("activation with mismatched event scope succeeded")
	}
	if repository.saves != 1 {
		t.Fatalf("mismatched activation wrote aggregate: saves=%d", repository.saves)
	}
}

func TestRuntimeControllerApplyInternalEventPersistsOwnerFactOnce(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision, beforeSaves := initialized.StateRevision, repository.saves
	fact := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment,
		Code:  "internal-event-regression", Signature: "internal-event-regression", Message: "owner-local test fact",
	}
	committed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventFailureObserved, Scope: RuntimeInternalEventScope{WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{FailureFact: &fact},
	})
	if err != nil {
		t.Fatalf("ApplyInternalEvent: %v", err)
	}
	if committed.Aggregate.StateRevision != beforeRevision+1 || repository.saves != beforeSaves+1 {
		t.Fatalf("internal event did not perform exactly one transition: aggregate=%+v saves=%d before=%d", committed.Aggregate, repository.saves, beforeSaves)
	}
	if committed.Aggregate.Facts.Failure == nil || committed.Aggregate.Facts.Failure.Code != fact.Code {
		t.Fatalf("owner fact was not persisted through internal event: %+v", committed.Aggregate.Facts.Failure)
	}
	duplicate, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventFailureObserved, Scope: RuntimeInternalEventScope{WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{FailureFact: &fact},
	})
	if err != nil {
		t.Fatalf("duplicate terminal internal event: %v", err)
	}
	if duplicate.Aggregate.StateRevision != committed.Aggregate.StateRevision || repository.saves != beforeSaves+1 {
		t.Fatalf("duplicate terminal fact rewrote aggregate: aggregate=%+v saves=%d", duplicate.Aggregate, repository.saves)
	}
}

func TestRuntimeControllerOwnerValidationRecipeExhaustionIsStableTypedTerminal(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OriginalClass: "semantic_feasibility", OriginalCode: "validation_recipe_exhausted",
		OriginalMessage: "sealed validation recipe is unavailable", Signature: "validation-recipe-exhausted/test",
	}
	failed, err := controller.failSemanticContract(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "semantic_owner_feasibility_infeasible", diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Decision.Terminal == nil || failed.Decision.Terminal.Code != "semantic_contract_failed" || failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible || failed.Facts.Failure.Class == runtimecontract.FailureClassNoProgressExhausted {
		t.Fatalf("validation recipe exhaustion did not become its typed terminal: %+v", failed)
	}
	if failed.Facts.Failure.OriginalDiagnostic == nil || failed.Facts.Failure.OriginalDiagnostic.OriginalCode != diagnostic.OriginalCode {
		t.Fatalf("validation recipe diagnostic was not preserved: %+v", failed.Facts.Failure)
	}
}

func TestRuntimeControllerRejectsValidationRecipeCategoryOutsideBlockedValidation(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("fixture decision = %s, want state change", aggregate.Decision.State)
	}
	if _, err := controller.recordFeasibilityFact(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeFeasibilityFact{
		FocusID: aggregate.Facts.Focus.ID, State: aggregate.Decision.State,
		Category: runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable,
		Reason:   "recipe unavailable",
	}); err == nil {
		t.Fatal("state-change authority accepted validation recipe terminal category")
	}
	if repository.saves != 1 {
		t.Fatalf("invalid feasibility category mutated aggregate: saves=%d", repository.saves)
	}
}

// C9 / T20: assessment observations are concurrent cross-owner facts, but
// their derived repair authority is persisted only inside the matching SM-2
// aggregate. The same controller must therefore never retain another
// workspace's claimed target, closure, coverage, or retry ledger.
func TestRuntimeControllerAssessmentRepairAuthorityIsScopedAcrossConcurrentWorkspaces(t *testing.T) {
	ctx := context.Background()
	repository := &scopedControllerRepository{aggregates: make(map[string]RuntimeAggregate)}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	type scenario struct {
		workspaceID common.WorkspaceID
		runID       common.RunID
		focusID     string
		coverageID  common.CoverageWorkItemID
		targetRef   string
	}
	scenarios := []scenario{
		{workspaceID: "ws-repair-a", runID: "run-repair-a", focusID: "focus-repair-a", coverageID: "cwi-repair-a", targetRef: "a.go"},
		{workspaceID: "ws-repair-b", runID: "run-repair-b", focusID: "focus-repair-b", coverageID: "cwi-repair-b", targetRef: "b.go"},
	}
	type outcome struct {
		scenario  scenario
		aggregate RuntimeAggregate
		err       error
	}
	results := make(chan outcome, len(scenarios))
	for _, current := range scenarios {
		current := current
		go func() {
			facts := controllerFacts()
			facts.WorkspaceID, facts.RunID = current.workspaceID, current.runID
			seed := facts.DefinitionSeed.Clone()
			seed.CoverageItems[0].ID = current.coverageID
			seed.Slices[0].CoverageWorkItemIDs = []common.CoverageWorkItemID{current.coverageID}
			seed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(seed)
			facts.DefinitionSeed = seed
			facts.Slices = append([]runtimecontract.RuntimeAcceptanceSlice(nil), seed.Slices...)
			facts.Focus.ID = current.focusID
			facts.Focus.RequireAssessment = true
			facts.Focus.RequireExecutableBehaviorEvidence = true
			facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
			facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{current.coverageID}
			facts.Focus.TargetRefs = []string{current.targetRef}
			facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
			facts.Focus.ValidationRecipeID = runtimeManagedValidationEvidencePolicy().ID
			facts.Focus.RequiredCheckRefs = []string{"check/" + runtimeManagedValidationEvidencePolicy().ID}
			facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
			facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{
				ID: current.coverageID, SliceID: seed.CoverageItems[0].SliceID,
				SourceScope: seed.CoverageItems[0].SourceScope, SourceID: seed.CoverageItems[0].SourceID,
				TargetText: seed.CoverageItems[0].TargetText, State: runtimecontract.RuntimeCoverageItemStateOpen,
			}}
			facts = runtimeFactsWithManagedValidationRecipe(facts)
			runtimeTestSealCurrentPolicies(&facts)
			facts.DefinitionFocuses = nil
			facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
			definition := controllerDefinition()
			definition.CapabilityContract = append(definition.CapabilityContract, runtimecontract.RuntimeBlockKindManagedValidationSession)
			if _, err := controller.initialize(ctx, facts, definition, nil); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			semantic, err := controller.EnsureSemanticContractSealedForOperationTest(ctx, current.workspaceID, current.runID)
			if err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			facts = semantic.Facts
			current.focusID = facts.Focus.ID
			if len(facts.Focus.CoverageEvidenceBindings) != 1 {
				results <- outcome{scenario: current, err: fmt.Errorf("semantic focus has %d bindings, want one", len(facts.Focus.CoverageEvidenceBindings))}
				return
			}
			bindingID := facts.Focus.CoverageEvidenceBindings[0].ID
			stateCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "state-" + current.focusID, Kind: runtimecontract.StepKindFileWrite,
				BindingRefs: []string{bindingID}, TargetRefs: []string{current.targetRef}, Parameters: runtimeFileWriteParameters("state"),
			}}}}}
			if _, err := controller.BeginCandidateAttemptForTest(ctx, current.workspaceID, current.runID, 0, "state-"+current.focusID); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			if err := RecordFixtureCandidateFeasibilities(ctx, controller, current.workspaceID, current.runID, 0, stateCandidate); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			stateSealed, err := controller.AcceptCandidateAttempt(ctx, current.workspaceID, current.runID, 0, stateCandidate)
			if err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			if stateSealed.ActiveDispatch == nil {
				results <- outcome{scenario: current, err: fmt.Errorf("state-change seal has no active dispatch")}
				return
			}
			if len(stateSealed.ActiveDispatch.OperationExecutions) != 1 {
				results <- outcome{scenario: current, err: fmt.Errorf("state-change seal has %d executions", len(stateSealed.ActiveDispatch.OperationExecutions))}
				return
			}
			stateOperationID := stateSealed.ActiveDispatch.OperationExecutions[0].OperationID
			stateEvidence := ExecutionEvidence{
				EventID: "state-" + current.focusID, WorkspaceID: current.workspaceID, RunID: current.runID,
				DispatchID: stateSealed.ActiveDispatch.ID, AuthoritySnapshotID: stateSealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
				OperationID: stateOperationID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted, NoOp: true,
			}
			if err := attachRuntimeTestScopedOwnerReceiptResult(repository, &stateEvidence); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			stateOperation, found := candidateOperationForEvidence(stateSealed.ActiveDispatch.Candidate, stateOperationID)
			if !found || stateEvidence.Receipt == nil {
				results <- outcome{scenario: current, err: fmt.Errorf("state-change no-op has no exact sealed operation/receipt")}
				return
			}
			stateEvidence.Status = ExecutionEvidenceStatusRejected
			stateEvidence.Failure = runtimeOperationEffectMismatchFailure(
				stateSealed.Decision,
				stateSealed.ActiveDispatch.OperationExecutions[0],
				stateOperation,
				*stateEvidence.Receipt,
			)
			if stateEvidence.Failure == nil || stateEvidence.Failure.Code != runtimeStateChangeNoOpWithoutProvenanceCode {
				results <- outcome{scenario: current, err: fmt.Errorf("state-change no-op did not derive bounded provenance failure: %+v", stateEvidence.Failure)}
				return
			}
			stateCommitted, err := controller.commitEvidence(ctx, stateEvidence)
			if err != nil || stateCommitted.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
				if err == nil {
					err = fmt.Errorf("state change selected %q instead of validation: terminal=%+v failure=%+v", stateCommitted.Aggregate.Decision.State, stateCommitted.Aggregate.Decision.Terminal, stateCommitted.Aggregate.Facts.Failure)
				}
				results <- outcome{scenario: current, err: err}
				return
			}
			validationCandidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "validation-" + current.focusID, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
				BindingRefs: []string{bindingID}, TargetRefs: []string{current.targetRef}, Parameters: runtimeManagedValidationParameters(),
			}}}}}
			if _, err := controller.BeginCandidateAttemptForTest(ctx, current.workspaceID, current.runID, 0, "validation-"+current.focusID); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			if err := RecordFixtureCandidateFeasibilities(ctx, controller, current.workspaceID, current.runID, 0, validationCandidate); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			validationSealed, err := controller.AcceptCandidateAttempt(ctx, current.workspaceID, current.runID, 0, validationCandidate)
			if err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			if validationSealed.ActiveDispatch == nil || len(validationSealed.ActiveDispatch.OperationExecutions) != 1 {
				results <- outcome{scenario: current, err: fmt.Errorf("validation seal has no unique execution")}
				return
			}
			validationProof, found := runtimecontract.CoverageEvidenceProofForKind(facts.Focus.CoverageEvidenceBindings[0], runtimecontract.EvidenceKindValidation)
			if !found || len(validationProof.ValidationCheckAttestations) == 0 {
				results <- outcome{scenario: current, err: fmt.Errorf("validation binding has no exact checks: %+v", facts.Focus.CoverageEvidenceBindings[0])}
				return
			}
			protocolRef := validationProof.ValidationCheckAttestations[0].ObservationProtocolRef
			nativeChecks := make([]string, 0, len(validationProof.ValidationCheckAttestations))
			for _, attestation := range validationProof.ValidationCheckAttestations {
				if attestation.ObservationProtocolRef != protocolRef {
					results <- outcome{scenario: current, err: fmt.Errorf("validation binding has mixed observation protocols: %+v", validationProof.ValidationCheckAttestations)}
					return
				}
				nativeChecks = append(nativeChecks, attestation.NativeCheckRef)
			}
			validationEvidence := ExecutionEvidence{
				EventID: "validation-" + current.focusID, WorkspaceID: current.workspaceID, RunID: current.runID,
				DispatchID: validationSealed.ActiveDispatch.ID, AuthoritySnapshotID: validationSealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
				OperationID: validationSealed.ActiveDispatch.OperationExecutions[0].OperationID, Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusAccepted,
				Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{
					RecipeID: runtimeManagedValidationEvidencePolicy().ID, ObservationProtocolRef: protocolRef,
					ExecutedChecks: append([]string(nil), nativeChecks...), PassedChecks: append([]string(nil), nativeChecks...),
				}},
			}
			if err := attachRuntimeTestScopedOwnerReceiptResult(repository, &validationEvidence); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			validationCommitted, err := controller.commitEvidence(ctx, validationEvidence)
			if err != nil || validationCommitted.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
				if err == nil {
					err = fmt.Errorf("incomplete validation selected %q instead of assessment: terminal=%+v failure=%+v", validationCommitted.Aggregate.Decision.State, validationCommitted.Aggregate.Decision.Terminal, validationCommitted.Aggregate.Facts.Failure)
				}
				results <- outcome{scenario: current, err: err}
				return
			}
			if _, err := controller.BeginCandidateAttemptForTest(ctx, current.workspaceID, current.runID, 0, "assessment-"+current.focusID); err != nil {
				results <- outcome{scenario: current, err: fmt.Errorf("begin assessment attempt: %w", err)}
				return
			}
			sealed, err := controller.AdmitAndSealCandidateAttemptForTest(ctx, current.workspaceID, current.runID, 0, runtimecontract.RuntimeCandidate{
				Assessment: &runtimecontract.AssessmentCandidate{
					FocusID: current.focusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{current.coverageID},
					Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "implementation remains incomplete",
					Gap: &runtimecontract.RuntimeAssessmentGap{
						CoverageWorkItemIDs:  []common.CoverageWorkItemID{current.coverageID},
						MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
						TargetRefs:           []string{"untrusted-" + current.targetRef},
						Reason:               "provider observation only",
					},
				},
			})
			if err != nil {
				results <- outcome{scenario: current, err: fmt.Errorf("seal assessment candidate: %w", err)}
				return
			}
			if sealed.Aggregate.ActiveDispatch == nil {
				results <- outcome{scenario: current, err: fmt.Errorf("seal assessment candidate: %w: admission=%s failure=%+v decision=%s", common.ErrConflict, sealed.Admission.Disposition, sealed.Admission.Failure, sealed.Aggregate.Decision.State)}
				return
			}
			committed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
				Kind: RuntimeInternalEventAssessmentObserved, Scope: RuntimeInternalEventScope{WorkspaceID: current.workspaceID, RunID: current.runID}, Authority: &RuntimeInternalAuthorityPayload{},
			})
			if err != nil {
				err = fmt.Errorf("commit assessment observation: %w", err)
			}
			results <- outcome{scenario: current, aggregate: committed.Aggregate, err: err}
		}()
	}

	for range scenarios {
		result := <-results
		if result.err != nil {
			t.Fatalf("workspace %s assessment transition: %v", result.scenario.workspaceID, result.err)
		}
		directive := result.aggregate.Facts.RepairDirective
		if result.aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || directive == nil || directive.Closure == nil ||
			directive.FocusID != result.scenario.focusID || !sameRuntimeTargetRefs(directive.TargetRefs, []string{result.scenario.targetRef}) ||
			!coverageIDsIntersect(directive.CoverageWorkItemIDs, []common.CoverageWorkItemID{result.scenario.coverageID}) {
			t.Fatalf("workspace %s repair authority leaked or was not canonical: aggregate=%+v", result.scenario.workspaceID, result.aggregate)
		}
		if sameRuntimeTargetRefs(directive.TargetRefs, []string{"untrusted-" + result.scenario.targetRef}) {
			t.Fatalf("workspace %s provider target became repair authority: %+v", result.scenario.workspaceID, directive)
		}
		loaded, loadErr := controller.Load(ctx, result.scenario.workspaceID, result.scenario.runID)
		if loadErr != nil || loaded.Facts.RepairDirective == nil || loaded.Facts.RepairDirective.Closure == nil ||
			loaded.Facts.RepairDirective.Closure.ID != directive.Closure.ID {
			t.Fatalf("workspace %s persisted repair authority = %+v err=%v", result.scenario.workspaceID, loaded.Facts.RepairDirective, loadErr)
		}
	}
}

func TestRuntimeControllerReconcileMissingSealedProviderCeilingReturnsToPrerequisiteWithoutCandidateBudget(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	requirement := runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		MinimumBytes: 96 * 1024, Reason: "provider ceiling required",
	}
	requirement = normalizeRuntimeRequirements([]runtimecontract.RuntimeRequirement{requirement})[0]
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: runtimePrerequisiteID(requirement), Kind: requirement.Kind, OwnerModuleID: requirement.OwnerModuleID,
		MinimumBytes: requirement.MinimumBytes, Reason: requirement.Reason,
	}
	retainedSatisfied := make(map[string]struct{}, len(aggregate.Facts.Prerequisites.Satisfied))
	for _, value := range aggregate.Facts.Prerequisites.Satisfied {
		retainedSatisfied[value.ID] = struct{}{}
	}
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
	aggregate.Facts.Resources.ProviderCeiling = nil
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}

	reconciled, err := controller.reconcileAuthorityBeforeCandidate(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatalf("reconcileAuthorityBeforeCandidate: %v", err)
	}
	if reconciled.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		len(reconciled.CandidateAttempts) != 0 || reconciled.Facts.Repair.RepairAttempts != 0 ||
		reconciled.Facts.Recovery.ProviderTransportAttempts != 0 || reconciled.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("missing provider ceiling consumed a candidate or recovery budget: %+v", reconciled)
	}
	providerPending := false
	for _, value := range reconciled.Facts.Prerequisites.Pending {
		providerPending = providerPending || value.ID == prerequisite.ID
	}
	providerSatisfied := false
	retainedSatisfiedAfter := make(map[string]struct{}, len(reconciled.Facts.Prerequisites.Satisfied))
	for _, value := range reconciled.Facts.Prerequisites.Satisfied {
		providerSatisfied = providerSatisfied || value.ID == prerequisite.ID
		retainedSatisfiedAfter[value.ID] = struct{}{}
	}
	if !providerPending || providerSatisfied || !reflect.DeepEqual(retainedSatisfiedAfter, retainedSatisfied) || reconciled.Facts.Resources.ProviderCeiling != nil {
		t.Fatalf("missing provider ceiling was not returned to its sealed prerequisite: %+v", reconciled.Facts)
	}
	if violations := InspectRuntimeAuthorityInvariants(reconciled).Violations; len(violations) != 0 {
		t.Fatalf("reconciled provider ceiling retains authority violations: %+v", violations)
	}
}

func TestRuntimeControllerRestartReusesCommittedOwnerTargetSelection(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/unique_workspace_entry"
	facts.Focus.RequireTargetGrounding = true

	initialized, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, value := range initialized.Facts.Prerequisites.Pending {
		if value.Kind == runtimecontract.PrerequisiteKindTargetSelection {
			prerequisite = value
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("initial target-selection prerequisite missing: %+v", initialized.Facts.Prerequisites)
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable", SelectorRef: "file/unique_workspace_entry",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"implementation.go"}, CandidateTargetRefs: []string{"implementation.go"}, CandidateCount: 1,
		ObservedRevision: "tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v1"}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	committed, err := controller.commitPrerequisite(ctx, facts.WorkspaceID, facts.RunID, PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: &observation})
	if err != nil {
		t.Fatal(err)
	}
	if committed.Facts.Resources.TargetSelection == nil || committed.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("committed target selection = %+v", committed.Facts.Resources.TargetSelection)
	}
	saves := repository.saves
	restarted, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != saves || restarted.StateRevision != committed.StateRevision || restarted.Facts.Resources.TargetSelection == nil || restarted.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("restart did not reuse committed selection: restarted=%+v saves=%d", restarted, repository.saves)
	}
	if _, err := controller.commitPrerequisite(ctx, facts.WorkspaceID, facts.RunID, PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: &observation}); err == nil || repository.saves != saves {
		t.Fatalf("duplicate target selection changed persisted state: saves=%d err=%v", repository.saves, err)
	}
}

func TestCommitPrerequisiteAdvancesSatisfiedFocusAfterFreshnessReacquisition(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus = runtimecontract.CoverageFocus{
		ID: "focus-satisfied", SliceID: "slice-satisfied", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-satisfied"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite},
	}
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "focus-next", SliceID: "slice-next", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-next"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite},
	}}
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
		{ID: "cwi-satisfied", SliceID: "slice-satisfied", SourceScope: "dodItem", SourceID: "dod-satisfied", TargetText: "deliver satisfied", State: runtimecontract.RuntimeCoverageItemStateOpen},
		{ID: "cwi-next", SliceID: "slice-next", SourceScope: "dodItem", SourceID: "dod-next", TargetText: "deliver next", State: runtimecontract.RuntimeCoverageItemStateOpen},
	}
	facts.Slices = []runtimecontract.RuntimeAcceptanceSlice{
		{ID: "slice-satisfied", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-satisfied"}},
		{ID: "slice-next", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-next"}},
	}
	facts.DefinitionSeed.CoverageItems = []runtimecontract.RuntimeCoverageItem{
		{ID: "cwi-satisfied", SliceID: "slice-satisfied", SourceScope: "dodItem", SourceID: "dod-satisfied", TargetText: "deliver satisfied"},
		{ID: "cwi-next", SliceID: "slice-next", SourceScope: "dodItem", SourceID: "dod-next", TargetText: "deliver next"},
	}
	facts.DefinitionSeed.Slices = facts.Slices
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	facts.Focus.TargetRefs = []string{"target.txt"}
	requirements := []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "target.txt", Reason: "fresh target"}}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("initialized aggregate = %+v, want state change", aggregate.Decision)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "satisfied-focus-state-change"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "satisfy-current-focus", Kind: runtimecontract.StepKindFileWrite,
		BindingRefs: []string{facts.Focus.CoverageEvidenceBindings[0].ID}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("satisfied"),
	}}}}}
	if err := RecordFixtureCandidateFeasibilities(context.Background(), controller, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	// Make the already sealed target observation stale before reducing the
	// durable receipt. SM-2 must retain the receipt while File reacquires the
	// prerequisite; assessment and focus advancement resume afterward.
	repository.aggregate.Facts.Resources.TargetSnapshots = nil
	repository.aggregate.Facts = PreflightRuntimeFacts(repository.aggregate.Facts, requirements)
	evidence := ExecutionEvidence{
		EventID: "accepted-satisfied", WorkspaceID: "ws-v2", RunID: "run-v2",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, repository.aggregate, &evidence)
	evidenceCommit, err := controller.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = evidenceCommit.Aggregate
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(aggregate.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("initialized aggregate = %+v", aggregate)
	}

	committed, err := controller.commitPrerequisite(context.Background(), "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: aggregate.Facts.Prerequisites.Pending[0].ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "target.txt", Content: "fresh", ContentHash: "fresh-hash", ContentState: runtimecontract.SnapshotContentStateComplete},
	})
	if err != nil {
		t.Fatal(err)
	}
	if committed.Decision.State != runtimecontract.ConvergenceStateNeedAssessment ||
		!coverageIDsIntersect(committed.Facts.Focus.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-satisfied"}) {
		t.Fatalf("fresh prerequisite did not resume current assessment: %+v", committed)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "satisfied-focus-assessment"); err != nil {
		t.Fatal(err)
	}
	assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: committed.Facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-satisfied"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "current focus is satisfied",
	}}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, assessment); err != nil {
		t.Fatal(err)
	}
	advanced, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if !coverageIDsIntersect(advanced.Facts.Focus.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-next"}) ||
		len(advanced.Facts.RemainingFocuses) != 0 || advanced.Decision.Focus.ID != advanced.Facts.Focus.ID ||
		advanced.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningOrderAccepted {
		t.Fatalf("assessment did not advance the reacquired satisfied focus: %+v", advanced)
	}
}

func TestRuntimeControllerInitializeRejectsPersistedOperationContractRevisionMismatch(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	repository.aggregate.Facts.Activation.ExecutionContract.OperationContractRevision = "operation-contract-obsolete"

	_, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	var mismatch *runtimecontract.RuntimeOperationContractRevisionMismatchError
	if !errors.As(err, &mismatch) || !errors.Is(err, runtimecontract.ErrRuntimeOperationContractRevisionMismatch) {
		t.Fatalf("revision mismatch err = %v", err)
	}
	if mismatch.PersistedRevision != "operation-contract-obsolete" || repository.saves != 1 {
		t.Fatalf("mismatch=%+v saves=%d", mismatch, repository.saves)
	}
}

func TestRuntimeControllerInitializeRejectsPersistedMissingOperationContractRevision(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	repository.aggregate.Facts.Activation.ExecutionContract.OperationContractRevision = ""

	_, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	var mismatch *runtimecontract.RuntimeOperationContractRevisionMismatchError
	if !errors.As(err, &mismatch) || mismatch.PersistedRevision != "" || repository.saves != 1 {
		t.Fatalf("missing revision err=%v mismatch=%+v saves=%d", err, mismatch, repository.saves)
	}
}

func TestRuntimeControllerInitializeRejectsDefinitionDrift(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	if _, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	facts.DefinitionRevision = "definition-v2-changed"
	definition := controllerDefinition()
	definition.Revision = facts.DefinitionRevision
	facts.DefinitionSeed.DefinitionRevision = facts.DefinitionRevision
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	facts = runtimeTestPrepareUnboundActivationFacts(t, facts)
	_, err := controller.initialize(context.Background(), facts, definition, nil)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("definition drift err = %v, want conflict", err)
	}
}

func TestRuntimeControllerInitializeRejectsPersistedDefinitionContentDrift(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	definition := controllerDefinition()
	if _, err := controller.initialize(context.Background(), facts, definition, nil); err != nil {
		t.Fatal(err)
	}
	definition.GoalDefinition = "被替换的目标"
	_, err := controller.initialize(context.Background(), facts, definition, nil)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("definition content drift err = %v, want conflict", err)
	}
}

func TestRuntimeControllerInitializeAcceptsAdvancedFocusOnRestart(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	definition := controllerDefinition()
	initialized, err := controller.initialize(context.Background(), facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository.aggregate.Facts.Focus.ID = "focus-v2-next"
	repository.aggregate.Decision.Focus.ID = "focus-v2-next"

	restarted, err := controller.initialize(context.Background(), facts, definition, nil)
	if err != nil {
		t.Fatalf("advanced-focus restart: %v", err)
	}
	if restarted.Facts.Focus.ID != "focus-v2-next" || restarted.StateRevision != initialized.StateRevision {
		t.Fatalf("restart replaced progressed aggregate: %+v", restarted)
	}
}

func TestRuntimeControllerCommitsPrerequisiteAndReducesAtomically(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindManifest}})
	if err != nil {
		t.Fatal(err)
	}
	prerequisiteID := aggregate.Facts.Prerequisites.Pending[0].ID
	progressed, err := controller.commitPrerequisite(context.Background(), "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: prerequisiteID,
		Manifest:       &runtimecontract.WorkspaceManifest{Revision: "manifest-v1", TargetRefs: []string{"target.txt"}},
	})
	if err != nil {
		t.Fatalf("commitPrerequisite: %v", err)
	}
	if progressed.StateRevision != 2 || progressed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("progressed aggregate = %+v", progressed)
	}
	if progressed.Decision.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("prerequisite progress inherited retry attempt %d", progressed.Decision.AuthoritySnapshot.Attempt)
	}
}

func TestRuntimeControllerRejectsPersistedHistoricalFocusCorruptionBeforeCandidate(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}

	// 纯 SM-2 在协调写集中退休旧 focus；已持久化的非法前像不能通过候选前协调静默修复。
	// 同时保留退休、预算和目标隔离正例，验证拒绝路径不修改仓储。
	facts := controllerFacts()
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "implementation"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/profile_declared_candidates"
	facts.Focus.TargetSelectorOwnerRef = "profile-bootstrap"
	facts.Focus.TargetSelectorSubjectRef = "manager"
	facts.Focus.TargetSelectorCandidateRefs = []string{"internal/lsp/manager.go"}
	facts.Focus.RequireTargetGrounding = true
	facts.Prerequisites = runtimecontract.PrerequisiteState{}
	preparedAggregate := runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, controllerDefinition(), nil)
	for {
		var manifestPrerequisite runtimecontract.RuntimePrerequisite
		for _, prerequisite := range preparedAggregate.Facts.Prerequisites.Pending {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindManifest {
				manifestPrerequisite = prerequisite
				break
			}
		}
		if manifestPrerequisite.ID == "" {
			break
		}
		preparedAggregate, err = controller.commitPrerequisite(ctx, facts.WorkspaceID, facts.RunID, PrerequisiteResult{
			PrerequisiteID: manifestPrerequisite.ID,
			Manifest: &runtimecontract.WorkspaceManifest{
				Revision: "tree-current", TargetRefs: []string{"internal/lsp/manager.go"},
			},
		})
		if err != nil {
			t.Fatalf("commit current workspace manifest: %v", err)
		}
	}
	if preparedAggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("prepared decision = %+v", preparedAggregate.Decision)
	}
	preparedBefore := preparedAggregate.CloneRaw()
	savesBeforePrepared := repository.saves
	unchanged, err := controller.reconcileAuthorityBeforeCandidate(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || !reflect.DeepEqual(unchanged, preparedBefore) || repository.saves != savesBeforePrepared {
		t.Fatalf("合法当前 focus 协调应原样返回且不消耗 CAS: err=%v saves=%d", err, repository.saves)
	}
	preparedFacts := preparedAggregate.Facts
	preparedDecision := preparedAggregate.Decision
	retainedSatisfied := make(map[string]struct{}, len(preparedFacts.Prerequisites.Satisfied))
	for _, prerequisite := range preparedFacts.Prerequisites.Satisfied {
		retainedSatisfied[prerequisite.ID] = struct{}{}
	}

	historicalSelection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "selection-old-go-mod", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "module_definition", SelectorRef: "file/profile_declared_candidates",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"go.mod"},
		CandidateTargetRefs: []string{"go.mod"}, CandidateCount: 1, ObservedRevision: "tree-old",
	}
	historicalSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(historicalSelection)
	historicalPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: historicalSelection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-module", OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	historicalObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: historicalPrerequisite.ID, Kind: historicalPrerequisite.Kind, OwnerModuleID: historicalPrerequisite.OwnerModuleID,
		ContentHash: historicalSelection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: "tree-old",
		}},
	}
	historicalObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(historicalObservation)
	preparedFacts.Prerequisites.Satisfied = append(preparedFacts.Prerequisites.Satisfied, historicalPrerequisite)
	preparedFacts.Resources.TargetSelection = &historicalSelection
	preparedFacts.Resources.TargetSnapshots = append(preparedFacts.Resources.TargetSnapshots, runtimecontract.TargetSnapshot{
		TargetRef: "go.mod", ContentHash: "go-mod-old", ContentState: runtimecontract.SnapshotContentStateComplete,
	})
	preparedFacts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{historicalObservation}
	preparedAggregate.Facts = preparedFacts
	preparedAggregate.Decision = preparedDecision
	repository.aggregate = &preparedAggregate
	revisionBeforeReconcile := preparedAggregate.StateRevision

	if err := preparedAggregate.Validate(); err == nil {
		t.Fatal("历史 focus 损坏 fixture 意外符合持久化合同")
	}
	before := preparedAggregate.CloneRaw()
	savesBefore := repository.saves
	if _, err := controller.reconcileAuthorityBeforeCandidate(ctx, facts.WorkspaceID, facts.RunID); err == nil || !strings.Contains(err.Error(), "reconciliation preimage invariant") {
		t.Fatalf("持久化损坏前像未在协调入口拒绝: %v", err)
	}
	if repository.saves != savesBefore || !reflect.DeepEqual(*repository.aggregate, before) {
		t.Fatal("拒绝损坏前像仍修改了持久化材料、revision 或预算")
	}

	// 在写入任何 after-image 前，原 SM-2 纯归约仍必须隔离历史目标并完整退休旧观察。
	reconciled := before.CloneRaw()
	reconciled.Facts, reconciled.Decision, err = reduceRuntimeAggregateWithDirective(reconciled, reconciled.Facts, &reconciled.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatalf("reduce historical focus authority: %v", err)
	}
	reconciled.StateRevision++
	if err := reconciled.Validate(); err != nil {
		t.Fatalf("退休后的纯 after-image 不合法: %v", err)
	}
	if reconciled.StateRevision != revisionBeforeReconcile+1 || reconciled.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		len(reconciled.CandidateAttempts) != 0 || reconciled.Facts.Repair.RepairAttempts != 0 ||
		reconciled.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("authority reconciliation consumed candidate or recovery state: %+v", reconciled)
	}
	historicalRetired := false
	for _, prerequisite := range reconciled.Facts.Prerequisites.Retired {
		historicalRetired = historicalRetired || prerequisite.ID == historicalPrerequisite.ID
	}
	historicalObservationRetired := false
	for _, observation := range reconciled.Facts.Resources.RetiredPrerequisiteObservations {
		historicalObservationRetired = historicalObservationRetired || observation.ObservationID == historicalObservation.ObservationID
	}
	retainedSatisfiedAfter := make(map[string]struct{}, len(reconciled.Facts.Prerequisites.Satisfied))
	for _, prerequisite := range reconciled.Facts.Prerequisites.Satisfied {
		retainedSatisfiedAfter[prerequisite.ID] = struct{}{}
	}
	if reconciled.Facts.Resources.TargetSelection != nil || !reflect.DeepEqual(retainedSatisfiedAfter, retainedSatisfied) ||
		len(reconciled.Facts.Resources.PrerequisiteObservations) != 0 || !historicalRetired || !historicalObservationRetired {
		t.Fatalf("historical authority was not retired: %+v", reconciled.Facts)
	}
	if violations := InspectRuntimeAuthorityInvariants(reconciled).Violations; len(violations) != 0 {
		t.Fatalf("reconciled aggregate retains authority violations: %+v", violations)
	}
	if !reflect.DeepEqual(reconciled.Decision.RequiredOutput.TargetRefs, []string{"internal/lsp/manager.go"}) ||
		reflect.DeepEqual(reconciled.Decision.RequiredOutput.TargetRefs, historicalSelection.TargetRefs) {
		t.Fatalf("historical selection rewrote the sealed semantic target: %+v", reconciled.Decision.RequiredOutput)
	}
}

func TestRuntimeControllerRecordsInfeasibleCandidateAsReducerFact(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	aggregate, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	progressed, err := controller.recordFeasibilityFact(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeFeasibilityFact{
		FocusID: aggregate.Facts.Focus.ID, State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOperationKinds: []string{"file/write"}, MissingCapabilityKinds: []string{"file/write"}, Reason: "owner capability unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Decision.State != runtimecontract.ConvergenceStateFailed || progressed.Decision.Terminal == nil || progressed.Decision.Terminal.Code != "capability_unavailable" || progressed.Facts.Feasibility == nil || len(progressed.CandidateAttempts) != 0 {
		t.Fatalf("feasibility transition = %+v", progressed)
	}
}

func TestRuntimeControllerRecordsFailureFactBeforeTerminalDecision(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.recordFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed",
		Signature: "runtime-drive-failed", Message: "authority transaction mismatch",
	})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Facts.Failure == nil || terminal.Facts.Failure.Code != "runtime_drive_failed" ||
		terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "runtime_contract_defect" {
		t.Fatalf("failure fact terminal = %+v", terminal)
	}
	saves := repository.saves
	replayed, err := controller.recordFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Message: "replayed drive failure",
	})
	if err != nil || repository.saves != saves || replayed.StateRevision != terminal.StateRevision {
		t.Fatalf("terminal failure replay = %+v saves=%d err=%v", replayed, repository.saves, err)
	}
}

func TestRuntimeControllerOwnsDispatchAndEvidenceCommit(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	// This unit test exercises only the post-semantic dispatch/evidence path.
	// Semantic proof receipt commit must deterministically hand off to its
	// required assessment; completion belongs to the separate assessment tests.
	facts := controllerFacts()
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
	}}}}}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if err := RecordFixtureCandidateFeasibilities(context.Background(), controller, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatalf("AcceptCandidateAttempt: %v", err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	commitEvidence := ExecutionEvidence{
		EventID: "event-1", WorkspaceID: "ws-v2", RunID: "run-v2",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: operationID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, repository.aggregate, &commitEvidence)
	result, err := controller.commitEvidence(context.Background(), commitEvidence)
	if err != nil || result.Disposition != CommitDispositionCommitted {
		t.Fatalf("commitEvidence = %+v err=%v", result, err)
	}
	if result.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("next action = %s", result.Aggregate.Decision.State)
	}
}

func TestRuntimeControllerRejectsExecutableSealWithoutPersistedOwnerFeasibility(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "inventory-revision",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	inventory = runtimeTestOperationAdmissibilityForOwner(t, initialized.Facts, runtimecontract.RuntimeModuleFile)
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
	}}}}}
	candidate = runtimeTestBindCandidateToFacts(t, repository.aggregate.Facts, candidate)
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate); err == nil || !strings.Contains(err.Error(), "no current persisted feasibility result") {
		t.Fatalf("sealed executable candidate without owner feasibility: %v", err)
	}
	if _, err := controller.recordCandidateProposal(context.Background(), "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestCandidateOperationID(t, *repository.aggregate.CandidateAttempts[len(repository.aggregate.CandidateAttempts)-1].Proposal, 0)
	if _, err := controller.recordCandidateFeasibility(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: operationID, AdmissibilitySnapshotID: inventory.SnapshotID,
		CandidateManifestHash: runtimeTestPendingCandidateManifestHash(t, controller, "ws-v2", "run-v2", 0), Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
	}); err != nil {
		t.Fatal(err)
	}
	if sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate); err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("sealed candidate with persisted owner feasibility = %+v err=%v", sealed, err)
	}
}

func TestRuntimeControllerCommitsTypedAssessmentBeforeCompletion(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimeTestBindCandidateToFacts(t, facts, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
	}}}}})
	if err := RecordFixtureCandidateFeasibilities(context.Background(), controller, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	commitEvidence := ExecutionEvidence{
		EventID: "event-1", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: operationID,
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, repository.aggregate, &commitEvidence)
	evidence, err := controller.commitEvidence(context.Background(), commitEvidence)
	if err != nil || evidence.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("assessment authority = %+v err=%v", evidence.Aggregate.Decision, err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: evidence.Aggregate.Facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
		Summary: "current focus is satisfied", EvidenceRefs: []string{"event-1"},
	}}); err != nil {
		t.Fatal(err)
	}
	completed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Decision.State != runtimecontract.ConvergenceStateComplete || len(completed.Facts.Assessments) != 1 || completed.Facts.Assessments[0].Disposition != runtimecontract.AssessmentDispositionSatisfied {
		t.Fatalf("completed assessment aggregate = %+v", completed)
	}
}

func TestRuntimeControllerAssessmentGapDeltaResetsWindowButRetainsRemainingGap(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.ID = "focus-cwi-a"
	facts.Focus.SliceID = "slice-cwi-a"
	facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-a"}
	facts.Focus.TargetRefs = []string{"a.txt"}
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "focus-cwi-b", SliceID: "slice-cwi-b", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-b"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"b.txt"}, RequireAssessment: true,
	}}
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
		{ID: "cwi-a", SliceID: "slice-cwi-a", SourceScope: "dodItem", SourceID: "dod-a", State: runtimecontract.RuntimeCoverageItemStateOpen},
		{ID: "cwi-b", SliceID: "slice-cwi-b", SourceScope: "dodItem", SourceID: "dod-b", State: runtimecontract.RuntimeCoverageItemStateOpen},
	}
	facts.Slices = []runtimecontract.RuntimeAcceptanceSlice{
		{ID: "slice-cwi-a", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"}},
		{ID: "slice-cwi-b", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-b"}},
	}
	facts.DefinitionSeed.CoverageItems = []runtimecontract.RuntimeCoverageItem{
		{ID: "cwi-a", SliceID: "slice-cwi-a", SourceScope: "dodItem", SourceID: "dod-a", TargetText: "deliver a"},
		{ID: "cwi-b", SliceID: "slice-cwi-b", SourceScope: "dodItem", SourceID: "dod-b", TargetText: "deliver b"},
	}
	facts.DefinitionSeed.Slices = facts.Slices
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "state-change-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimeTestBindCandidateToFacts(t, facts, runtimecontract.RuntimeCandidate{
		StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-both", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"},
			TargetRefs: []string{"a.txt"}, Parameters: runtimeFileWriteParameters("state"),
		}}}},
	})
	if err := RecordFixtureCandidateFeasibilities(ctx, controller, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	evidence := ExecutionEvidence{
		EventID: "event-state", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: operationID,
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, repository.aggregate, &evidence)
	committed, err := controller.commitEvidence(ctx, evidence)
	if err != nil || committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("state evidence did not reach assessment: aggregate=%+v err=%v", committed.Aggregate, err)
	}
	// Model a persisted repairable gap for cwi-a. Its accepted assessment moves
	// to cwi-b, which must receive a fresh repair ledger rather than inheriting
	// cwi-a's counters, shape history, gap, or repair directive.
	repository.aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: repository.aggregate.Facts.Focus.ID, SliceID: repository.aggregate.Facts.Focus.SliceID,
		RepairAttempts: 2, ValidationOnlyRetries: 1, NoProgressCount: 2, LastCycleID: "prior-cycle",
		SignatureCounts:           []runtimecontract.RuntimeFailureSignatureCount{{Signature: "prior-gap", Count: 2}},
		NoProgressOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{Operation: runtimecontract.RuntimeOperation{Kind: runtimecontract.StepKindFileWrite}}},
		ActiveGap:                 &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"}, MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, Reason: "cwi-a remains open"},
	}
	repository.aggregate.Facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-cwi-a", FocusID: repository.aggregate.Facts.Focus.ID, Trigger: runtimecontract.RuntimeRepairTriggerAssessmentGap,
		RequiredState:         runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-a"},
		TargetRefs:            []string{"a.txt"},
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
		Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: committed.Aggregate.Facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"},
			Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "cwi-a satisfied",
			Items: []runtimecontract.RuntimeAssessmentItemFact{
				{CoverageWorkItemID: "cwi-a", Satisfied: true, EvidenceRefs: []string{"event-state"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	progressed, err := controller.commitAssessment(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	ledger := progressed.Facts.Repair
	if !coverageIDsIntersect(progressed.Facts.Focus.CoverageWorkItemIDs, []common.CoverageWorkItemID{"cwi-b"}) ||
		ledger.FocusID != progressed.Facts.Focus.ID || ledger.SliceID != "slice-cwi-b" ||
		ledger.RepairAttempts != 0 || ledger.ValidationOnlyRetries != 0 || ledger.NoProgressCount != 0 || len(ledger.SignatureCounts) != 0 ||
		len(ledger.NoProgressOperationShapes) != 0 || ledger.LastCycleID != "" || ledger.ActiveGap != nil || ledger.LastProgressRevision == "" ||
		progressed.Facts.RepairDirective != nil {
		t.Fatalf("focus transition retained cwi-a repair authority in cwi-b: facts=%+v", progressed.Facts)
	}
	for _, coverage := range progressed.Facts.Coverage {
		if coverage.ID == "cwi-a" && coverage.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled {
			t.Fatalf("satisfied cwi-a was not settled atomically: %+v", coverage)
		}
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || reloaded.StateRevision != progressed.StateRevision || !reflect.DeepEqual(reloaded.Facts.Repair, ledger) {
		t.Fatalf("restart did not preserve CWI-isolated repair transition: aggregate=%+v err=%v", reloaded, err)
	}
}

func TestRuntimeControllerTerminalAssessmentClaimCannotOverrideOwnerCoverage(t *testing.T) {
	controller := prepareAssessmentController(t, runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionTerminalRejection, Summary: "producer claims terminal", EvidenceRefs: []string{"event-1"},
		Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: []string{"unknown-evidence"}}},
	})
	completed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatalf("CommitAssessment observation ref err = %v", err)
	}
	if completed.Decision.State != runtimecontract.ConvergenceStateComplete || len(completed.Facts.Assessments) != 1 ||
		completed.Facts.Assessments[0].ClaimedDisposition != runtimecontract.AssessmentDispositionTerminalRejection ||
		completed.Facts.Assessments[0].Disposition != runtimecontract.AssessmentDispositionSatisfied ||
		completed.Facts.Assessments[0].Items[0].EvidenceRefs[0] != "unknown-evidence" {
		t.Fatalf("terminal producer claim changed canonical result or observation: %+v", completed)
	}
}

func TestRuntimeControllerDoesNotRewriteAssessmentItemEvidenceRefs(t *testing.T) {
	controller := prepareAssessmentController(t, runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "satisfied", EvidenceRefs: []string{"event-1"},
		Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true}},
	})
	completed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	canonicalReceiptRef := ""
	if len(completed.Facts.Evidence.Accepted) == 1 {
		canonicalReceiptRef = string(completed.Facts.Evidence.Accepted[0].ID)
	}
	if completed.Decision.State != runtimecontract.ConvergenceStateComplete || canonicalReceiptRef == "" || len(completed.Facts.Coverage) != 1 ||
		len(completed.Facts.Coverage[0].EvidenceRefs) != 1 || completed.Facts.Coverage[0].EvidenceRefs[0] != canonicalReceiptRef {
		t.Fatalf("assessment item evidence inheritance = %+v", completed)
	}
	if len(completed.Facts.Assessments) != 1 || len(completed.Facts.Assessments[0].Items) != 1 ||
		len(completed.Facts.Assessments[0].Items[0].EvidenceRefs) != 0 ||
		len(completed.Facts.Assessments[0].EvidenceRefs) != 1 || completed.Facts.Assessments[0].EvidenceRefs[0] != "event-1" {
		t.Fatalf("assessment fact did not preserve original observation refs: %+v", completed.Facts.Assessments)
	}
}

func prepareAssessmentController(t *testing.T, assessment runtimecontract.AssessmentCandidate) *RuntimeController {
	t.Helper()
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "state-change-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimeTestBindCandidateToFacts(t, facts, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
	}}}}})
	if err := RecordFixtureCandidateFeasibilities(context.Background(), controller, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	commitEvidence := ExecutionEvidence{
		EventID: "event-1", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: operationID,
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, repository.aggregate, &commitEvidence)
	evidence, err := controller.commitEvidence(context.Background(), commitEvidence)
	if err != nil || evidence.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("prepare assessment evidence = %+v err=%v", evidence, err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	assessment.FocusID = evidence.Aggregate.Facts.Focus.ID
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{Assessment: &assessment}); err != nil {
		t.Fatal(err)
	}
	return controller
}

func TestRuntimeControllerCandidateRetryIsPersistedAndLimitedToTwoAttempts(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Capabilities.GovernanceKinds = []string{"decision/request"}
	if _, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	firstPending, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatalf("pending attempt replay should be idempotent: %v", err)
	}
	if repository.saves != 6 {
		t.Fatalf("pending replay performed another save: %d", repository.saves)
	}
	firstFailed, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassSchema))
	if err != nil {
		t.Fatal(err)
	}
	if firstFailed.Decision.AuthoritySnapshot.ID != firstPending.Decision.AuthoritySnapshot.ID || firstFailed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("attempt 0 failure changed contract: %+v", firstFailed.Decision)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "different-prefix"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different-prefix retry err = %v, want conflict", err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	exhausted, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassProvider, Reason: "provider unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exhausted.Decision.State != runtimecontract.ConvergenceStateFailed || exhausted.Decision.Terminal == nil || exhausted.Decision.Terminal.Code != "candidate_retry_exhausted" {
		t.Fatalf("exhausted decision = %+v", exhausted.Decision)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("third request err = %v, want conflict", err)
	}
}

func TestRuntimeControllerCandidateRetryExhaustionClosesAtomically(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassSchema)); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	savesBeforeFailure := repository.saves

	terminal, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid", Reason: "unexpected EOF",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != savesBeforeFailure+1 {
		t.Fatalf("attempt failure saves = %d, want one atomic save after %d", repository.saves, savesBeforeFailure)
	}
	if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "candidate_retry_exhausted" || hasPendingCandidateAttempt(terminal.CandidateAttempts) {
		t.Fatalf("attempt 1 failure did not atomically close terminal authority: %+v", terminal)
	}
	last := terminal.CandidateAttempts[len(terminal.CandidateAttempts)-1]
	if last.Attempt != 1 || last.State != runtimecontract.RuntimeCandidateAttemptStateFailed || last.Failure == nil || last.Failure.Code != "candidate_schema_invalid" {
		t.Fatalf("attempt 1 record = %+v", last)
	}
}

func TestRuntimeControllerTerminalDriveFailureClosesPendingCandidate(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	savesBeforeFailure := repository.saves

	terminal, err := controller.recordFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed",
		Signature: "runtime-drive-failed", Message: "candidate transaction could not be closed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != savesBeforeFailure+1 {
		t.Fatalf("drive failure saves = %d, want one atomic save after %d", repository.saves, savesBeforeFailure)
	}
	if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "runtime_contract_defect" || hasPendingCandidateAttempt(terminal.CandidateAttempts) {
		t.Fatalf("drive failure did not close terminal authority and candidate transaction: %+v", terminal)
	}
	attempt := terminal.CandidateAttempts[len(terminal.CandidateAttempts)-1]
	if attempt.State != runtimecontract.RuntimeCandidateAttemptStateFailed || attempt.Failure == nil || attempt.Failure.Class != runtimecontract.RuntimeCandidateFailureClassInternal || attempt.Failure.Code != "runtime_drive_failed" {
		t.Fatalf("terminal candidate closure = %+v", attempt)
	}
}

func TestRuntimeControllerReopensInvalidatedAttemptZeroWithoutCandidateDebt(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	transaction := aggregate.Decision.AuthorityTransaction()
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: "operation-admissibility", ObservationID: "observation-v1",
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
	}
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
		Transaction:         transaction, Attempt: 0, StablePrefixHash: "stable-prefix", State: runtimecontract.RuntimeCandidateAttemptStateInvalidated,
		Stale: &stale,
	}}
	bindRuntimeCandidateAttemptForTest(t, &aggregate, 0)
	aggregate.PrerequisiteInvalidations = []runtimecontract.RuntimePrerequisiteInvalidationRecord{{
		Signature: signature, PrerequisiteIDs: []string{stale.PrerequisiteID}, ObservationIDs: []string{stale.ObservationID}, Stale: &stale,
	}}
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}

	beforeRevision := aggregate.StateRevision
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources((aggregate.Facts).Resources))
	if err != nil {
		t.Fatal(err)
	}
	logicalRequestID := aggregate.CandidateAttempts[0].LogicalRequestID
	if _, err := controller.beginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "drifted-prefix", manifest, logicalRequestID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stable-prefix drift err=%v, want conflict", err)
	}
	if repository.saves != 0 || repository.aggregate.StateRevision != beforeRevision {
		t.Fatalf("stable-prefix drift mutated aggregate: revision=%d saves=%d", repository.aggregate.StateRevision, repository.saves)
	}
	reopened, err := controller.beginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix", manifest, logicalRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.StateRevision != beforeRevision+1 || len(reopened.CandidateAttempts) != 1 {
		t.Fatalf("invalidated attempt was not atomically reopened: %+v", reopened)
	}
	record := reopened.CandidateAttempts[0]
	if record.Attempt != 0 || record.State != runtimecontract.RuntimeCandidateAttemptStatePending || record.StablePrefixHash != "stable-prefix" ||
		record.Transaction != reopened.Decision.AuthorityTransaction() || record.Proposal != nil || len(record.Feasibilities) != 0 || record.Failure != nil || record.Stale != nil {
		t.Fatalf("reopened attempt = %+v", record)
	}
	if len(reopened.PrerequisiteInvalidations) != 1 || reopened.PrerequisiteInvalidations[0].Signature != signature {
		t.Fatalf("freshness audit was not retained: %+v", reopened.PrerequisiteInvalidations)
	}
	if repository.saves != 1 {
		t.Fatalf("reopen saves=%d, want 1", repository.saves)
	}
	duplicate, err := controller.beginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix", manifest, logicalRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.StateRevision != reopened.StateRevision || repository.saves != 1 || len(duplicate.CandidateAttempts) != 1 {
		t.Fatalf("duplicate reopen was not idempotent: aggregate=%+v saves=%d", duplicate, repository.saves)
	}
}

func TestRuntimeControllerCandidateFailureDoesNotResealPreExistingExecutionFailure(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "base_hash_mismatch",
		Signature: "patch-base-hash", Message: "patch baseHash mismatch",
	}
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 1}
	// Establish the execution failure on the activation aggregate, then seal the
	// exact semantic generation before candidate handling. The failure remains
	// SM-2 state; it is not a license to start with handwritten bindings.
	initialized := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, controllerDefinition(), nil)
	pending, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	failed, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassContract))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.AuthoritySnapshot != pending.Decision.AuthoritySnapshot || failed.Decision.AuthoritySnapshot != initialized.Decision.AuthoritySnapshot {
		t.Fatalf("attempt 0 failure re-sealed authority: initialized=%+v pending=%+v failed=%+v", initialized.Decision.AuthoritySnapshot, pending.Decision.AuthoritySnapshot, failed.Decision.AuthoritySnapshot)
	}
	if failed.Facts.Failure == nil || failed.Facts.Failure.Signature != "patch-base-hash" {
		t.Fatalf("attempt 0 failure replaced execution failure: %+v", failed.Facts.Failure)
	}
	retried, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if len(retried.CandidateAttempts) != 2 || retried.CandidateAttempts[0].AuthorityContractID == "" || retried.CandidateAttempts[0].AuthorityContractID != retried.CandidateAttempts[1].AuthorityContractID {
		t.Fatalf("candidate retry did not preserve authority contract identity: %+v", retried.CandidateAttempts)
	}
}

func TestRuntimeControllerRejectsRetryWhenPersistedContractIDIsMissing(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	failed, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassContract))
	if err != nil {
		t.Fatal(err)
	}
	failed.CandidateAttempts[0].AuthorityContractID = ""
	resealed, err := ReduceRuntimeState(failed.Facts, &failed.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	failed.Decision = resealed
	failed.StateRevision++
	repository.aggregate = &failed
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); err == nil {
		t.Fatal("candidate retry accepted persisted attempt without authority contract id")
	}
}

func TestRuntimeControllerCandidateExhaustionWithGovernanceCapabilityIsTerminal(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	if _, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	// N6: provider-class complete rejection is rejected_final. Attempt 0 must
	// seal terminal in the same CAS; governance capability must not open a
	// correction attempt or governance ticket for it. Reducer maps the
	// misalignment FailureFact class to terminal code runtime_contract_defect
	// while preserving the sealed provider code on the attempt/fact.
	terminal, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProvider, Reason: "provider unavailable"})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "runtime_contract_defect" ||
		terminal.Facts.Failure == nil || terminal.Facts.Failure.Code != "provider" ||
		len(terminal.CandidateAttempts) != 1 || terminal.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed ||
		terminal.CandidateAttempts[0].Failure == nil || terminal.CandidateAttempts[0].Failure.Code != "provider" ||
		len(terminal.GovernanceRequests) != 0 || terminal.ActiveDispatch != nil {
		t.Fatalf("rejected_final provider failure = action=%s terminal=%+v fact=%+v attempt=%+v gov=%d", terminal.Decision.State, terminal.Decision.Terminal, terminal.Facts.Failure, terminal.CandidateAttempts, len(terminal.GovernanceRequests))
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("attempt 1 after rejected_final err=%v, want conflict", err)
	}
}

func TestRuntimeControllerCandidateExhaustionWithoutGovernanceIsTerminal(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassContract)); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: "invalid operation again"})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "candidate_retry_exhausted" {
		t.Fatalf("terminal decision = %+v", terminal.Decision)
	}
}

func TestRuntimeControllerInternalCandidateFailureIsImmediatelyTerminal(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: "transaction mismatch"})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "runtime_contract_defect" {
		t.Fatalf("terminal decision = %+v", terminal.Decision)
	}
}

func TestFailInfeasibleValidationCandidateDoesNotFabricateStateChangeRepair(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts = runtimeFactsWithManagedValidationRecipe(facts)
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	facts = initialized.Facts
	if initialized.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("initial decision = %+v, want validation", initialized.Decision)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), facts.WorkspaceID, facts.RunID, 0, "validation-prefix"); err != nil {
		t.Fatal(err)
	}
	continued, err := controller.failCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassContract))
	if err != nil {
		t.Fatal(err)
	}
	if continued.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("infeasible validation fabricated state-change repair: %+v", continued.Decision)
	}
	if continued.Facts.Failure != nil || continued.Facts.Repair.RepairAttempts != 0 || continued.Facts.Repair.ActiveGap != nil {
		t.Fatalf("candidate infeasibility created repair authority: %+v", continued.Facts)
	}
	if len(continued.CandidateAttempts) != 1 || continued.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed {
		t.Fatalf("validation candidate failure was not retained: %+v", continued.CandidateAttempts)
	}
}

func TestFailInfeasibleValidationCandidateDoesNotConsumeRepairBudget(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{
		MaxRepairAttemptsPerFocus: 1, MaxValidationOnlyRetriesPerFocus: 1,
		MaxAcceptanceSliceWorkItems: 1, MaxFocuses: 1, MaxCandidateContextItems: 1,
	}
	facts = runtimeFactsWithManagedValidationRecipe(facts)
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	facts = initialized.Facts
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), facts.WorkspaceID, facts.RunID, 0, "validation-prefix"); err != nil {
		t.Fatal(err)
	}
	continued, err := controller.failCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassContract))
	if err != nil {
		t.Fatal(err)
	}
	if continued.Facts.Repair.RepairAttempts != 0 || continued.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("candidate infeasible consumed repair budget: %+v", continued)
	}
}

func TestFailInfeasibleValidationCandidateKeepsValidationWhenPolicyAllowsAlternative(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts = runtimeFactsWithManagedValidationRecipe(facts)
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	facts = initialized.Facts
	if initialized.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("initial decision = %+v, want validation", initialized.Decision)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), facts.WorkspaceID, facts.RunID, 0, "validation-prefix"); err != nil {
		t.Fatal(err)
	}
	continued, err := controller.failCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassContract))
	if err != nil {
		t.Fatal(err)
	}
	if continued.Decision.State != runtimecontract.ConvergenceStateNeedValidation || continued.Facts.Failure != nil {
		t.Fatalf("alternative-validation policy changed authority unexpectedly: %+v", continued)
	}
}

func TestFailInfeasibleValidationCommandDiagnosticKeepsSameContractRetry(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts = runtimeFactsWithManagedValidationRecipe(facts)
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	facts = initialized.Facts
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), facts.WorkspaceID, facts.RunID, 0, "validation-prefix"); err != nil {
		t.Fatal(err)
	}
	failure := runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassContract)
	failure.ValidationDiagnostic = &runtimecontract.ValidationFailureDiagnostic{
		SchemaVersion: runtimecontract.ValidationFailureDiagnosticSchemaVersion,
		Class:         runtimecontract.ValidationDiagnosticCommandInvalid, RecommendedEvidenceTargetType: runtimecontract.ValidationEvidenceTargetCommandContract,
	}
	continued, err := controller.failCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, failure)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Decision.AuthoritySnapshot.ID != initialized.Decision.AuthoritySnapshot.ID || continued.Decision.State != runtimecontract.ConvergenceStateNeedValidation || continued.Facts.Failure != nil || continued.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("command diagnostic changed SM-2 repair authority: %+v", continued)
	}
	if len(continued.CandidateAttempts) != 1 || continued.CandidateAttempts[0].Failure == nil || continued.CandidateAttempts[0].Failure.ValidationDiagnostic == nil {
		t.Fatalf("command diagnostic was not retained for same-contract retry: %+v", continued.CandidateAttempts)
	}
}

func TestFailInfeasibleValidationCommandDiagnosticDoesNotCreateImplementationRepair(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts = runtimeFactsWithManagedValidationRecipe(facts)
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, context.Background(), controller, facts, definition, nil)
	facts = initialized.Facts
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), facts.WorkspaceID, facts.RunID, 0, "validation-prefix"); err != nil {
		t.Fatal(err)
	}
	failure := runtimeCorrectableMissingOperationsFailure(runtimecontract.RuntimeCandidateFailureClassContract)
	failure.ValidationDiagnostic = &runtimecontract.ValidationFailureDiagnostic{
		SchemaVersion: runtimecontract.ValidationFailureDiagnosticSchemaVersion,
		Class:         runtimecontract.ValidationDiagnosticCommandInvalid, RecommendedEvidenceTargetType: runtimecontract.ValidationEvidenceTargetCommandContract,
	}
	continued, err := controller.failCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, failure)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Decision.State != runtimecontract.ConvergenceStateNeedValidation || continued.Facts.Failure != nil || continued.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("validation command diagnostic created implementation repair: %+v", continued)
	}
}

func controllerFacts() runtimecontract.RuntimeFacts {
	facts := runtimecontract.RuntimeFacts{
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired},
		},
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "runtime-evidence-00000000000000000000",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-v2", SliceID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"target.txt"},
		},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", SliceID: "slice-v2", SourceScope: "dodItem", SourceID: "dod-v2", TargetText: "deliver", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Slices:       []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"file/write"}},
	}
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy = runtimecontract.DefaultRuntimeRecoveryPolicy()
	facts.ExecutionBudgetPolicy = runtimecontract.DefaultRuntimeExecutionBudgetPolicy()
	facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{
		SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: facts.DefinitionRevision,
		PolicyHash: "policy-test", CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-v2", SliceID: "slice-v2", SourceScope: "dodItem", SourceID: "dod-v2", TargetText: "deliver"}},
		Slices: facts.Slices,
	}
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	facts.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{{
		ID: "definition-focus-v2", SliceID: "slice-v2", AssessmentScope: "dodItem", AssessmentID: "dod-v2", AssessmentText: "deliver", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-v2"),
	}}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "test-semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "test-semantic-revision",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	return facts
}

func runtimeManagedValidationEvidencePolicy() runtimecontract.RuntimeEvidencePolicy {
	return runtimecontract.RuntimeEvidencePolicy{
		ID: "test-managed-validation", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true",
		RecipeDeclared: true, RecipeArgs: []string{"workorch-validation"}, RecipeTimeoutMillis: 5_000,
	}
}

func runtimeManagedValidationEvidencePolicies() []runtimecontract.RuntimeEvidencePolicy {
	primary := runtimeManagedValidationEvidencePolicy()
	return []runtimecontract.RuntimeEvidencePolicy{
		primary,
		{
			ID: "test-managed-validation-alternative", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true",
			RecipeDeclared: true, RecipeArgs: []string{"workorch-validation-alternative"}, RecipeTimeoutMillis: 5_000,
		},
	}
}

func runtimeManagedValidationParameters() []runtimecontract.OperationParameter {
	id := runtimeManagedValidationEvidencePolicy().ID
	return []runtimecontract.OperationParameter{{Name: "recipeId", String: &id}}
}

// runtimeTestManagedValidationInventory represents the output of Shell
// owner preflight in unit fixtures.  Candidate tests must never obtain an
// executable recipe merely from activation policy declarations.
func runtimeTestManagedValidationInventory(prerequisiteID string) runtimecontract.RuntimeOperationAdmissibility {
	recipes := runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(
		runtimeManagedValidationEvidencePolicies(),
	)
	surfaces := make([]runtimecontract.RuntimeValidationSurface, 0, len(recipes))
	for _, recipe := range recipes {
		surfaces = append(surfaces, runtimecontract.RuntimeValidationSurface{
			ID: recipe.ID, OwnerModuleID: runtimecontract.RuntimeModuleShell,
			OperationKind:   runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness:       runtimecontract.RuntimeValidationSurfaceReady,
			InputSchemaHash: "test-managed-validation-input/v1", InputBindingRef: "test-managed-validation-input/" + recipe.ID,
			ProofApplicability: runtimecontract.RuntimeValidationProofNotDeclared,
			EffectScope:        runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		})
	}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID:     prerequisiteID,
		OwnerModuleID:      runtimecontract.RuntimeModuleShell,
		OperationKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationSurfaces: surfaces,
		ObservedRevision:   "shell-policy-test-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	return inventory
}

func runtimeTestManagedValidationPrerequisite(id string) runtimecontract.RuntimePrerequisite {
	return runtimecontract.RuntimePrerequisite{
		ID: id, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:        runtimecontract.RuntimeModuleShell,
		RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		Reason:               "test Shell owner preflight",
	}
}

// runtimeFactsWithManagedValidationRecipe is deliberately opt-in.  It models
// the result of a Shell-owner preflight only in tests that exercise managed
// validation; generic controller fixtures must not gain an unrelated Shell
// prerequisite or alter their state-change authority.
func runtimeFactsWithManagedValidationRecipe(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	prerequisite := runtimeTestManagedValidationPrerequisite("operation-admissibility-shell-managed-validation")
	facts.Activation.EvidencePolicies = runtimeManagedValidationEvidencePolicies()
	declareValidation := func(focus runtimecontract.CoverageFocus) runtimecontract.CoverageFocus {
		focus = focus.Normalized()
		if !hasRuntimeTestEvidenceKind(focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) {
			return focus
		}
		if hasRuntimeTestEvidenceKind(focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindStateChange) {
			for _, kind := range facts.Capabilities.StateChangeKinds {
				focus.AcceptedStepKinds = runtimeTestAppendUnique(focus.AcceptedStepKinds, kind)
			}
		}
		focus.AcceptedStepKinds = runtimeTestAppendUnique(focus.AcceptedStepKinds, runtimecontract.RuntimeBlockKindManagedValidationSession)
		if focus.ValidationRecipeID == "" {
			focus.ValidationRecipeID = runtimeManagedValidationEvidencePolicy().ID
		}
		if len(focus.RequiredCheckRefs) == 0 {
			focus.RequiredCheckRefs = []string{"check/" + focus.ValidationRecipeID}
		}
		return focus.Normalized()
	}
	facts.Focus = declareValidation(facts.Focus)
	for idx := range facts.RemainingFocuses {
		facts.RemainingFocuses[idx] = declareValidation(facts.RemainingFocuses[idx])
	}
	for idx := range facts.CompletedFocuses {
		facts.CompletedFocuses[idx] = declareValidation(facts.CompletedFocuses[idx])
	}
	for idx := range facts.DefinitionFocuses {
		declaration := facts.DefinitionFocuses[idx].Clone()
		if !hasRuntimeTestEvidenceKind(declaration.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) {
			continue
		}
		if hasRuntimeTestEvidenceKind(declaration.RequiredEvidenceKinds, runtimecontract.EvidenceKindStateChange) {
			for _, kind := range facts.Capabilities.StateChangeKinds {
				declaration.AcceptedStepKinds = runtimeTestAppendUnique(declaration.AcceptedStepKinds, kind)
			}
		}
		declaration.AcceptedStepKinds = runtimeTestAppendUnique(declaration.AcceptedStepKinds, runtimecontract.RuntimeBlockKindManagedValidationSession)
		if declaration.ValidationRecipeID == "" {
			declaration.ValidationRecipeID = runtimeManagedValidationEvidencePolicy().ID
		}
		if declaration.EvidenceDeclarationRef == "" || strings.Contains(declaration.EvidenceDeclarationRef, "*") {
			declaration.EvidenceDeclarationRef = runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(declaration.AssessmentScope, declaration.AssessmentID)
		}
		if len(declaration.RequiredCheckRefs) == 0 {
			declaration.RequiredCheckRefs = []string{"check/" + declaration.ValidationRecipeID}
		}
		facts.DefinitionFocuses[idx] = declaration.Normalized()
	}
	satisfied := facts.Prerequisites.Satisfied[:0]
	for _, existing := range facts.Prerequisites.Satisfied {
		if existing.ID == prerequisite.ID && existing.OwnerModuleID == prerequisite.OwnerModuleID {
			continue
		}
		satisfied = append(satisfied, existing)
	}
	facts.Prerequisites.Satisfied = append(satisfied, prerequisite)
	inventory := runtimeTestManagedValidationInventory(prerequisite.ID)
	inventory.TargetRefs = append([]string(nil), facts.Focus.TargetRefs...)
	inventory.TargetRefs = append(inventory.TargetRefs, facts.Focus.ProspectiveTargetRefs...)
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	catalogs := facts.Resources.OperationAdmissibilities[:0]
	for _, existing := range facts.Resources.OperationAdmissibilities {
		if existing.PrerequisiteID == inventory.PrerequisiteID && existing.OwnerModuleID == inventory.OwnerModuleID {
			continue
		}
		catalogs = append(catalogs, existing)
	}
	facts.Resources.OperationAdmissibilities = append(catalogs, inventory)
	if facts.DefinitionSeed.SchemaVersion != "" {
		facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
		bound, err := runtimeTestBindManagedValidationProofRequirements(facts)
		if err != nil {
			panic(fmt.Sprintf("bind managed-validation test inventory: %v", err))
		}
		facts = bound
	}
	return facts
}

func controllerDefinition() runtimecontract.RuntimeDefinitionContext {
	return runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-v2", GoalID: "goal-v2", GoalDefinition: "完成目标",
		DoDID: "dod-v2", DoDDefinition: "产生可验证交付物", CapabilityContract: []string{"file/write"},
	}
}
