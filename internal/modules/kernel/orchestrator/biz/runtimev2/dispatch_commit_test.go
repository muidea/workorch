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
	sealed, err := coordinator.PersistDispatch(context.Background(), repository.aggregate.Decision, candidate)
	if err != nil {
		t.Fatal(err)
	}
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
	sealed, err := coordinator.PersistDispatch(context.Background(), aggregate.Decision, validationCandidate())
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-validation-failed", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "validate-1", Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "build-failed", Message: "main redeclared", TargetRefs: []string{"main.go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || result.Aggregate.Decision.RequiredOutput.Kind != runtimecontract.RequiredOutputKindStateChange {
		t.Fatalf("validation failure decision = %+v, want atomic state change", result.Aggregate.Decision)
	}
}

func TestCoordinatorRestartRecoversPersistedDispatchWithoutProcessState(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	first, _ := NewDispatchCoordinator(repository)
	sealed, err := first.PersistDispatch(context.Background(), repository.aggregate.Decision, stateChangeCandidate())
	if err != nil {
		t.Fatal(err)
	}
	second, _ := NewDispatchCoordinator(repository)
	result, err := second.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-after-restart", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil || result.Disposition != CommitDispositionCommitted {
		t.Fatalf("restart commit = %+v err=%v", result, err)
	}
}

func TestDispatchCoordinatorCommitsMultiOperationDispatchOnlyAfterAllOperations(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: "patch/apply", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"}}},
		{Operation: runtimecontract.RuntimeOperation{ID: "write-2", Kind: "patch/apply", DependsOn: []string{"write-1"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"other.go"}}},
	}}}
	sealed, err := coordinator.PersistDispatch(context.Background(), repository.aggregate.Decision, candidate)
	if err != nil {
		t.Fatal(err)
	}
	first, err := coordinator.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-write-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-1",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.DispatchCompleted || first.Aggregate.ActiveDispatch == nil || len(first.Aggregate.ActiveDispatch.CompletedOperationIDs) != 1 {
		t.Fatalf("first operation prematurely completed dispatch: %+v", first)
	}
	if first.Aggregate.Decision.AuthoritySnapshot.ID != sealed.Decision.AuthoritySnapshot.ID {
		t.Fatalf("decision changed before dispatch completion")
	}
	second, err := coordinator.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-write-2", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-2",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
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
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	sealed, err := coordinator.PersistDispatch(context.Background(), aggregate.Decision, stateChangeCandidate())
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-focus-1", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
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
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-v2-00000000000000000000",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRefs: []string{"main.go"},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"patch/apply"}, ValidationKinds: []string{"validation/managed_session"}},
	}
}

func stateChangeCandidate() runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "patch-1", Kind: "patch/apply", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
	}}}}}
}

func validationCandidate() runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: "validation/managed_session", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
	}}}}}
}
