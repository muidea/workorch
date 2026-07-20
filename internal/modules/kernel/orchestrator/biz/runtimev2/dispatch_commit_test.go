package runtimev2

import (
	"context"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type durableRuntimeStateRepository struct {
	aggregate RuntimeAggregate
}

func (r *durableRuntimeStateRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return r.aggregate, nil
}

func (r *durableRuntimeStateRepository) Save(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	if r.aggregate.StateRevision != expectedRevision {
		return fmt.Errorf("%w: runtime aggregate revision changed", common.ErrConflict)
	}
	r.aggregate = aggregate
	return nil
}

func TestDispatchCommitAdvancesEvidenceAndReducerExactlyOnce(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	candidate := stateChangeCandidate()
	sealed := sealCandidateForTest(t, repository, candidate)
	if sealed.ActiveDispatch == nil || sealed.ActiveDispatch.Transaction != repository.aggregate.Decision.AuthorityTransaction() || sealed.ActiveDispatch.CandidateHash != runtimecontract.StableRuntimeCandidateHash(candidate) {
		t.Fatalf("sealed dispatch = %+v", sealed.ActiveDispatch)
	}
	stepInput := sealed.ActiveDispatch.StepInput()
	if stepInput.DispatchID == "" || stepInput.Transaction != sealed.ActiveDispatch.Transaction || stepInput.CandidateHash != sealed.ActiveDispatch.CandidateHash {
		t.Fatalf("dispatch step input did not persist complete transaction: %+v", stepInput)
	}
	evidence := ExecutionEvidence{
		EventID: "event-state-change-1", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted, SourceRef: "patch/1",
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Disposition != CommitDispositionCommitted || committed.Aggregate.EvidenceSequence != 1 || committed.Aggregate.Facts.EvidenceRevision != "evidence-v2-00000000000000000001" {
		t.Fatalf("commit result = %+v", committed)
	}
	if committed.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceValidation || committed.Aggregate.ActiveDispatch != nil || len(committed.Aggregate.CompletedDispatches) != 1 {
		t.Fatalf("aggregate did not advance atomically: %+v", committed.Aggregate)
	}
	stateRevision := committed.Aggregate.StateRevision
	duplicate, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != stateRevision || len(duplicate.Aggregate.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("duplicate commit changed state: %+v", duplicate)
	}
}

func TestStaleEvidenceOnlyEntersDiagnosticLineage(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	coordinator, _ := NewDispatchCoordinator(repository)
	currentFocus := repository.aggregate.Decision.Focus.ID
	currentRevision := repository.aggregate.Facts.EvidenceRevision
	result, err := coordinator.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-stale-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-old",
		AuthoritySnapshotID: "authority-old", OperationID: "patch-old", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitDispositionStale || len(result.Aggregate.Diagnostics) != 1 || result.Aggregate.Decision.Focus.ID != currentFocus || result.Aggregate.Facts.EvidenceRevision != currentRevision || len(result.Aggregate.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("stale evidence changed authority facts: %+v", result.Aggregate)
	}
	again, err := coordinator.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-stale-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-old",
		AuthoritySnapshotID: "authority-old", OperationID: "patch-old", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil || again.Disposition != CommitDispositionDuplicate || len(again.Aggregate.Diagnostics) != 1 {
		t.Fatalf("duplicate stale evidence = %+v err=%v", again, err)
	}
}

func TestRejectedValidationEvidenceAtomicallyReturnsToStateChange(t *testing.T) {
	aggregate := validationAggregate(t)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	commitEvidence := ExecutionEvidence{
		EventID: "event-validation-failed", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "validate-1", Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "build-failed", Message: "main redeclared", TargetRefs: []string{"main.go"}},
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &commitEvidence)
	result, err := coordinator.CommitEvidence(context.Background(), commitEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || result.Aggregate.Decision.RequiredOutput.Kind != runtimecontract.RequiredOutputKindStateChange {
		t.Fatalf("validation failure decision = %+v, want atomic state change", result.Aggregate.Decision)
	}
}

func TestRejectedTextReplaceConsumesRepairBudgetAndRequiresFreshTargetSnapshot(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 3}
	aggregate.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "main.go", Content: "package main\n", ContentHash: "old-hash", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	oldText := "func missing() {}"
	newText := "func present() {}"
	patchType := "text_replace"
	targetRef := "main.go"
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{targetRef},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "targetRef", String: &targetRef}, {Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		},
	}}}}}
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-patch-conflict", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "text_replace_not_found",
			Signature: "patch-text-replace-not-found", Message: "oldText not found", TargetRefs: []string{"main.go"},
		},
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &evidence)
	rejected, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Aggregate.Facts.Repair.RepairAttempts != 1 || rejected.Aggregate.Facts.Repair.ActiveGap == nil {
		t.Fatalf("repair ledger = %+v", rejected.Aggregate.Facts.Repair)
	}
	if rejected.Aggregate.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(rejected.Aggregate.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("rejected patch decision = %+v prerequisites=%+v", rejected.Aggregate.Decision, rejected.Aggregate.Facts.Prerequisites.Pending)
	}
	prerequisite := rejected.Aggregate.Facts.Prerequisites.Pending[0]
	if prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || prerequisite.SubjectRef != "main.go" || len(rejected.Aggregate.Facts.Resources.TargetSnapshots) != 0 {
		t.Fatalf("fresh snapshot prerequisite = %+v resources=%+v", prerequisite, rejected.Aggregate.Facts.Resources)
	}
	revision := rejected.Aggregate.StateRevision
	duplicate, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revision || duplicate.Aggregate.Facts.Repair.RepairAttempts != 1 {
		t.Fatalf("duplicate rejection changed repair authority: %+v", duplicate)
	}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := controller.CommitPrerequisite(context.Background(), "ws-1", "run-1", PrerequisiteResult{
		PrerequisiteID: prerequisite.ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{
			TargetRef: "main.go", Content: "package main\nfunc present() {}\n", ContentHash: "fresh-hash", ContentState: runtimecontract.SnapshotContentStateComplete,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || len(refreshed.Facts.Prerequisites.Pending) != 0 ||
		len(refreshed.Facts.Resources.TargetSnapshots) != 1 || refreshed.Facts.Resources.TargetSnapshots[0].ContentHash != "fresh-hash" {
		t.Fatalf("fresh snapshot did not return to same-focus repair: %+v", refreshed)
	}
}

func TestCoordinatorRestartRecoversPersistedDispatchWithoutProcessState(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	sealed := sealCandidateForTest(t, repository, stateChangeCandidate())
	second, _ := NewDispatchCoordinator(repository)
	commitEvidence := ExecutionEvidence{
		EventID: "event-after-restart", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &commitEvidence)
	result, err := second.CommitEvidence(context.Background(), commitEvidence)
	if err != nil || result.Disposition != CommitDispositionCommitted {
		t.Fatalf("restart commit = %+v err=%v", result, err)
	}
}

func TestDispatchCoordinatorCommitsMultiOperationDispatchOnlyAfterAllOperations(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: patchApplyTestOperation("write-1", "main.go")},
		{Operation: func() runtimecontract.RuntimeOperation {
			operation := patchApplyTestOperation("write-2", "other.go")
			operation.DependsOn = []string{"write-1"}
			return operation
		}()},
	}}}
	sealed := sealCandidateForTest(t, repository, candidate)
	firstEvidence := ExecutionEvidence{
		EventID: "event-write-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-1",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &firstEvidence)
	first, err := coordinator.CommitEvidence(context.Background(), firstEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if first.DispatchCompleted || first.Aggregate.ActiveDispatch == nil || len(first.Aggregate.ActiveDispatch.CompletedOperationIDs) != 1 {
		t.Fatalf("first operation prematurely completed dispatch: %+v", first)
	}
	if first.Aggregate.Decision.AuthoritySnapshot.ID != sealed.Decision.AuthoritySnapshot.ID {
		t.Fatalf("decision changed before dispatch completion")
	}
	secondEvidence := ExecutionEvidence{
		EventID: "event-write-2", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-2",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &secondEvidence)
	second, err := coordinator.CommitEvidence(context.Background(), secondEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if !second.DispatchCompleted || second.Aggregate.ActiveDispatch != nil || len(second.Aggregate.CompletedDispatches) != 1 {
		t.Fatalf("second operation did not complete dispatch: %+v", second)
	}
}

func TestDispatchCommitAdvancesToNextCoverageFocusDeterministically(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	aggregate.Facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "focus-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"other.go"},
	}}
	aggregate.Facts.Coverage = runtimecontract.InitialRuntimeCoverageFacts([]runtimecontract.RuntimeCoverageItem{{ID: "cwi-1"}, {ID: "cwi-2"}})
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	sealed := sealCandidateForTest(t, repository, stateChangeCandidate())
	commitEvidence := ExecutionEvidence{
		EventID: "event-focus-1", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &commitEvidence)
	result, err := coordinator.CommitEvidence(context.Background(), commitEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Facts.Focus.ID != "focus-2" || len(result.Aggregate.Facts.RemainingFocuses) != 0 ||
		result.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || result.Aggregate.Decision.Focus.ID != "focus-2" {
		t.Fatalf("next focus aggregate = %+v", result.Aggregate)
	}
	if len(result.Aggregate.Facts.Evidence.Accepted) != 1 || len(result.Aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs) != 1 ||
		result.Aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("focus-bound evidence = %+v", result.Aggregate.Facts.Evidence.Accepted)
	}
}

func stateChangeAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	facts := baseDispatchFacts()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	return RuntimeAggregate{StateRevision: 1, Definition: dispatchDefinition(), Facts: facts, Decision: decision}
}

func TestRuntimeAggregateRejectsTerminalDecisionWithPendingCandidateAttempt(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	transaction := aggregate.Decision.AuthorityTransaction()
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
		Transaction:         transaction,
		Attempt:             0,
		StablePrefixHash:    "pending-prefix",
		State:               runtimecontract.RuntimeCandidateAttemptStatePending,
	}}

	for _, terminal := range []runtimecontract.RuntimeDecision{
		sealedTerminalDecision(t, aggregate.Facts, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "failed"),
		sealedTerminalDecision(t, aggregate.Facts, runtimecontract.ConvergenceStateComplete, runtimecontract.RuntimeActionComplete, "complete"),
	} {
		aggregate.Decision = terminal
		if err := aggregate.Validate(); err == nil {
			t.Fatalf("terminal decision %+v accepted a pending candidate attempt", terminal)
		}
	}
}

func TestRuntimeAggregateRequiresReducerTransitionAfterCandidateAttemptOneFailure(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction)
	failure := &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_replays_rejected_shape", Reason: "replayed shape"}
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{
		{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, StablePrefixHash: "stable-prefix", State: runtimecontract.RuntimeCandidateAttemptStateFailed, Failure: failure},
		{AuthorityContractID: contractID, Transaction: transaction, Attempt: 1, StablePrefixHash: "stable-prefix", State: runtimecontract.RuntimeCandidateAttemptStateFailed, Failure: failure},
	}
	if err := aggregate.Validate(); err == nil {
		t.Fatal("same-authority attempt 1 failure was accepted without a reducer transition")
	}
	aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted", Signature: "candidate-retry-exhausted", Message: "replayed shape"}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("reduced exhausted candidate state rejected: %v", err)
	}
}

func sealedTerminalDecision(t *testing.T, facts runtimecontract.RuntimeFacts, state runtimecontract.ConvergenceState, action runtimecontract.RuntimeAction, code string) runtimecontract.RuntimeDecision {
	t.Helper()
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: state, Action: action, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{Kind: runtimecontract.RequiredOutputKindNone},
		Terminal:       &runtimecontract.TerminalOutcome{Code: code},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

// sealCandidateForTest builds a persisted post-seal fixture without retaining a
// production alternate dispatch entry point. Production code must use the
// RuntimeController candidate-attempt admission path.
func sealCandidateForTest(t *testing.T, repository *durableRuntimeStateRepository, candidate runtimecontract.RuntimeCandidate) RuntimeAggregate {
	t.Helper()
	aggregate := repository.aggregate
	dispatch, err := SealDispatchWithActivation(aggregate.Decision, candidate, aggregate.Facts.Activation)
	if err != nil {
		t.Fatal(err)
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
		Transaction:         transaction,
		Attempt:             0,
		StablePrefixHash:    "test-sealed-dispatch",
		State:               runtimecontract.RuntimeCandidateAttemptStateAccepted,
	})
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository.aggregate = aggregate
	return aggregate
}

func validationAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	facts := baseDispatchFacts()
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "accepted-state-change", Kind: runtimecontract.EvidenceKindStateChange, Revision: facts.EvidenceRevision}}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	return RuntimeAggregate{StateRevision: 1, Definition: dispatchDefinition(), Facts: facts, Decision: decision}
}

func dispatchDefinition() runtimecontract.RuntimeDefinitionContext {
	return runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal",
		DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply", "validation/managed_session"},
	}
}

func baseDispatchFacts() runtimecontract.RuntimeFacts {
	return runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-v2-00000000000000000000",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRefs: []string{"main.go"},
		},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"patch/apply"}, ValidationKinds: []string{"validation/managed_session"}},
	}
}

func stateChangeCandidate() runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "patch-1", Kind: "patch/apply", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"}, Parameters: patchApplyTestParameters("main.go"),
	}}}}}
}

func patchApplyTestOperation(id string, targetRef string) runtimecontract.RuntimeOperation {
	return runtimecontract.RuntimeOperation{
		ID: id, Kind: runtimecontract.RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{targetRef}, Parameters: patchApplyTestParameters(targetRef),
	}
}

func patchApplyTestParameters(targetRef string) []runtimecontract.OperationParameter {
	patchType := "file_write"
	content := "updated\n"
	return []runtimecontract.OperationParameter{
		{Name: "targetRef", String: &targetRef},
		{Name: "patchType", String: &patchType},
		{Name: "content", String: &content},
	}
}

func validationCandidate() runtimecontract.RuntimeCandidate {
	command := "true"
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: "validation/managed_session", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		Parameters: []runtimecontract.OperationParameter{{Name: "command", String: &command}},
	}}}}}
}

func TestAdjudicateEvidenceKindRejectsDurableMismatch(t *testing.T) {
	decision := runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionProduceStateChange}
	// Owner reports none while seal expected durable.
	_, err := adjudicateEvidenceKind(decision, runtimecontract.RuntimeEffectWorkspaceDurable, true, ExecutionEvidence{
		EventID: "e1", OperationID: "op-1", Status: ExecutionEvidenceStatusAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectNone, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "noop"},
	})
	if err == nil {
		t.Fatal("non-durable observed under state-change must fail adjudication")
	}
}

func TestSealDispatchCreatesOperationExecutionRecords(t *testing.T) {
	facts := controllerFacts()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("state"),
		},
	}}}}
	dispatch, err := SealDispatchWithActivation(decision, candidate, facts.Activation)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispatch.OperationExecutions) != 1 {
		t.Fatalf("executions = %+v", dispatch.OperationExecutions)
	}
	if dispatch.OperationExecutions[0].State != runtimecontract.OperationExecutionSealed {
		t.Fatalf("state = %s", dispatch.OperationExecutions[0].State)
	}
	if dispatch.OperationExecutions[0].ExpectedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		t.Fatalf("expected scope = %s", dispatch.OperationExecutions[0].ExpectedEffectScope)
	}
	if dispatch.OperationExecutions[0].ExecutionKey == "" {
		t.Fatal("execution key required")
	}
}
