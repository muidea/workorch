package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProspectiveAllOfReceiptSetSettlesExactlyOnceAfterRestart(t *testing.T) {
	repository, coordinator, operations := prospectiveAllOfDispatchFixture(t, false)
	firstEvidence := prospectiveAllOfEvidence(repository.aggregate, operations[0], "prospective-first", ExecutionEvidenceStatusAccepted, nil)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &firstEvidence)
	partial, err := coordinator.commitEvidence(context.Background(), firstEvidence)
	if err != nil {
		t.Fatal(err)
	}
	assertProspectiveCWIState(t, partial.Aggregate, runtimecontract.RuntimeCoverageItemStateOpen, runtimecontract.RuntimeCWIConvergenceStateBound)
	if partial.DispatchCompleted || partial.Aggregate.ActiveDispatch == nil || len(partial.Aggregate.Facts.Evidence.Accepted) != 1 || len(partial.Aggregate.OperationReceipts) != 1 || len(partial.Aggregate.CoverageProofLineage) != 1 {
		t.Fatalf("partial prospective receipt did not remain auditable and open: %+v", partial.Aggregate)
	}
	corrupted := partial.Aggregate
	corrupted.Facts.Coverage = append([]runtimecontract.RuntimeCoverageItemFact(nil), partial.Aggregate.Facts.Coverage...)
	corrupted.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
	corrupted.Facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
	if err := corrupted.Validate(); err == nil {
		t.Fatal("current-shape reload accepted a settled CWI with only a partial prospective receipt set")
	}

	// A fresh coordinator has no process-local credit state. It must read the
	// partial receipt/evidence lineage and continue the same sealed transaction.
	restarted, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	revisionBeforeDuplicate := repository.aggregate.StateRevision
	duplicate, err := restarted.commitEvidence(context.Background(), firstEvidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revisionBeforeDuplicate {
		t.Fatalf("partial receipt duplicate after restart = %+v err=%v", duplicate, err)
	}

	stale, err := restarted.commitEvidence(context.Background(), ExecutionEvidence{
		EventID: "prospective-stale", WorkspaceID: repository.aggregate.Facts.WorkspaceID, RunID: repository.aggregate.Facts.RunID,
		DispatchID: repository.aggregate.ActiveDispatch.ID, AuthoritySnapshotID: "stale-authority", OperationID: operations[1].ID,
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil || stale.Disposition != CommitDispositionStale {
		t.Fatalf("stale prospective evidence = %+v err=%v", stale, err)
	}
	assertProspectiveCWIState(t, stale.Aggregate, runtimecontract.RuntimeCoverageItemStateOpen, runtimecontract.RuntimeCWIConvergenceStateBound)

	secondEvidence := prospectiveAllOfEvidence(repository.aggregate, operations[1], "prospective-second", ExecutionEvidenceStatusAccepted, nil)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &secondEvidence)
	receiptComplete, err := restarted.commitEvidence(context.Background(), secondEvidence)
	if err != nil {
		t.Fatal(err)
	}
	assertProspectiveCWIState(t, receiptComplete.Aggregate, runtimecontract.RuntimeCoverageItemStateOpen, runtimecontract.RuntimeCWIConvergenceStateBound)
	if !receiptComplete.DispatchCompleted || receiptComplete.Aggregate.ActiveDispatch != nil || len(receiptComplete.Aggregate.CompletedDispatches) != 1 ||
		receiptComplete.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment ||
		len(receiptComplete.Aggregate.Facts.Evidence.Accepted) != 2 || len(receiptComplete.Aggregate.OperationReceipts) != 2 || len(receiptComplete.Aggregate.CoverageProofLineage) != 2 {
		t.Fatalf("complete prospective receipt set did not reach assessment with full audit lineage: %+v", receiptComplete.Aggregate)
	}
	refs, satisfied := canonicalAssessmentEvidenceForCoverage(receiptComplete.Aggregate, "cwi-1")
	if !satisfied || len(refs) != 2 {
		t.Fatalf("complete prospective receipt set did not establish assessment proof: refs=%v satisfied=%t", refs, satisfied)
	}
	completed := commitProspectiveSatisfiedAssessment(t, repository, refs)
	assertProspectiveCWIState(t, completed, runtimecontract.RuntimeCoverageItemStateSatisfied, runtimecontract.RuntimeCWIConvergenceStateSettled)
	revisionAfterCompletion := completed.StateRevision
	completedDuplicate, err := restarted.commitEvidence(context.Background(), secondEvidence)
	if err != nil || completedDuplicate.Disposition != CommitDispositionDuplicate || completedDuplicate.Aggregate.StateRevision != revisionAfterCompletion ||
		len(completedDuplicate.Aggregate.Facts.Evidence.Accepted) != 2 || len(completedDuplicate.Aggregate.CoverageProofLineage) != 2 {
		t.Fatalf("complete prospective duplicate changed collective credit: %+v err=%v", completedDuplicate, err)
	}
	if err := completedDuplicate.Aggregate.Validate(); err != nil {
		t.Fatalf("completed prospective aggregate failed reload invariant: %v", err)
	}
}

func TestProspectiveAllOfRejectedReceiptCannotSettleCWI(t *testing.T) {
	repository, coordinator, operations := prospectiveAllOfDispatchFixture(t, false)
	first := prospectiveAllOfEvidence(repository.aggregate, operations[0], "prospective-reject-first", ExecutionEvidenceStatusAccepted, nil)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &first)
	if _, err := coordinator.commitEvidence(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "owner_rejected", Signature: "owner-rejected-second", Message: "second target rejected",
	}
	second := prospectiveAllOfEvidence(repository.aggregate, operations[1], "prospective-reject-second", ExecutionEvidenceStatusRejected, failure)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &second)
	rejected, err := coordinator.commitEvidence(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	assertProspectiveCWIState(t, rejected.Aggregate, runtimecontract.RuntimeCoverageItemStateOpen, runtimecontract.RuntimeCWIConvergenceStateBound)
	if !rejected.DispatchCompleted || rejected.Aggregate.ActiveDispatch != nil || len(rejected.Aggregate.Facts.Evidence.Accepted) != 1 ||
		len(rejected.Aggregate.Facts.Evidence.Rejected) != 1 || len(rejected.Aggregate.OperationReceipts) != 2 || len(rejected.Aggregate.CoverageProofLineage) != 1 {
		t.Fatalf("rejected prospective member changed collective CWI credit or lost audit: %+v", rejected.Aggregate)
	}
}

func TestProspectiveAllOfAssessmentClosureUsesCollectiveReceiptGate(t *testing.T) {
	repository, coordinator, operations := prospectiveAllOfDispatchFixture(t, true)
	first := prospectiveAllOfEvidence(repository.aggregate, operations[0], "prospective-assessment-first", ExecutionEvidenceStatusAccepted, nil)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &first)
	partial, err := coordinator.commitEvidence(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if refs, satisfied := canonicalAssessmentEvidenceForCoverage(partial.Aggregate, "cwi-1"); satisfied || len(refs) != 0 {
		t.Fatalf("partial prospective receipt set passed assessment gate: refs=%v satisfied=%t", refs, satisfied)
	}

	second := prospectiveAllOfEvidence(repository.aggregate, operations[1], "prospective-assessment-second", ExecutionEvidenceStatusAccepted, nil)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &second)
	ready, err := coordinator.commitEvidence(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("complete prospective receipt set did not reach assessment: %+v", ready.Aggregate.Decision)
	}
	refs, satisfied := canonicalAssessmentEvidenceForCoverage(ready.Aggregate, "cwi-1")
	if !satisfied || len(refs) != 2 {
		t.Fatalf("complete prospective receipt set did not pass assessment gate: refs=%v satisfied=%t", refs, satisfied)
	}
	closed := commitProspectiveSatisfiedAssessment(t, repository, refs)
	assertProspectiveCWIState(t, closed, runtimecontract.RuntimeCoverageItemStateSatisfied, runtimecontract.RuntimeCWIConvergenceStateSettled)
	if closed.Decision.State != runtimecontract.ConvergenceStateComplete || closed.Decision.Terminal == nil {
		t.Fatalf("collective assessment did not close exactly once: %+v", closed.Decision)
	}
}

func prospectiveAllOfDispatchFixture(t *testing.T, requireAssessment bool) (*durableRuntimeStateRepository, *DispatchCoordinator, []runtimecontract.RuntimeOperation) {
	t.Helper()
	facts := baseDispatchFacts()
	facts.Focus.SliceID = "slice-prospective"
	facts.Coverage[0].SourceScope = "dodItem"
	facts.Coverage[0].SourceID = "dod-1"
	facts.Coverage[0].TargetText = "deliver the complete prospective target set"
	facts.Coverage[0].SliceID = facts.Focus.SliceID
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Focus.RequireAssessment = requireAssessment
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetSelectorRef = "file/workspace_deliverable"
	facts.Focus.TargetSelectorOwnerRef = runtimecontract.RuntimeModuleFile
	facts.Focus.TargetSelectorSubjectRef = "workspace_deliverable"
	facts.Focus.TargetSelectorCandidateRefs = []string{"go.mod", "workorch_test.go"}
	facts.Focus.ProspectiveTargetRefs = []string{"go.mod", "workorch_test.go"}
	facts.Focus.RequireTargetGrounding = true
	facts.Slices = []runtimecontract.RuntimeAcceptanceSlice{{ID: facts.Focus.SliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}}
	facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{
		SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: facts.DefinitionRevision,
		PolicyHash: "policy-test",
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{
			ID: "cwi-1", SliceID: facts.Focus.SliceID, SourceScope: facts.Coverage[0].SourceScope,
			SourceID: facts.Coverage[0].SourceID, TargetText: facts.Coverage[0].TargetText,
		}},
		Slices: facts.Slices,
	}
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	aggregate := RuntimeAggregate{ResultFeedbackVersion: runtimecontract.RuntimeResultFeedbackVersionCurrent, StateRevision: 1, Definition: dispatchDefinition(), Facts: facts}
	addCandidateAdmissionTargetPolicyFixture(&aggregate.Facts, runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeBlockKindPatchApply, "file_write", runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, nil)
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	sequence := aggregate.Decision.RequiredOutput.RequiredOperationSequence
	if len(sequence) != 2 {
		t.Fatalf("prospective fixture required sequence = %+v decision=%+v focus=%+v", sequence, aggregate.Decision, aggregate.Facts.Focus)
	}
	operationIDs := make(map[string]string, len(sequence))
	for index, requirement := range sequence {
		operationIDs[requirement.RequirementID] = fmt.Sprintf("prospective-write-%d", index+1)
	}
	operations := make([]runtimecontract.RuntimeOperation, 0, len(sequence))
	for _, requirement := range sequence {
		operation := patchApplyTestOperation(operationIDs[requirement.RequirementID], requirement.TargetRef)
		operation.RequiredOperationRef = requirement.RequirementID
		for _, dependencyRef := range requirement.DependsOnRequirementIDs {
			operation.DependsOn = append(operation.DependsOn, operationIDs[dependencyRef])
		}
		operations = append(operations, operation)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{}}
	for _, operation := range operations {
		candidate.StateChange.Operations = append(candidate.StateChange.Operations, runtimecontract.StateChangingOperation{Operation: operation})
	}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	catalog := aggregate.Facts.Resources.OperationAdmissibilities[0]
	feasibilities := make([]runtimecontract.RuntimeCandidateFeasibilityRecord, 0, len(operations))
	for _, operation := range operations {
		feasibilities = append(feasibilities, runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
			Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
		})
	}
	sealed := sealCandidateWithFeasibilitiesForTest(t, repository, candidate, feasibilities)
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	return repository, coordinator, candidateRuntimeOperations(sealed.ActiveDispatch.Candidate)
}

func commitProspectiveSatisfiedAssessment(t *testing.T, repository *durableRuntimeStateRepository, evidenceRefs []string) RuntimeAggregate {
	t.Helper()
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := repository.aggregate.Facts.WorkspaceID
	runID := repository.aggregate.Facts.RunID
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), workspaceID, runID, 0, "prospective-assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: repository.aggregate.Facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "all prospective targets are proven", EvidenceRefs: append([]string(nil), evidenceRefs...),
		Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-1", Satisfied: true, EvidenceRefs: append([]string(nil), evidenceRefs...)}},
	}}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), workspaceID, runID, 0, assessment); err != nil {
		t.Fatal(err)
	}
	closed, err := controller.commitAssessment(context.Background(), workspaceID, runID)
	if err != nil {
		t.Fatal(err)
	}
	return closed
}

func prospectiveAllOfEvidence(aggregate RuntimeAggregate, operation runtimecontract.RuntimeOperation, eventID string, status runtimecontract.RuntimeOperationExecutionStatus, failure *runtimecontract.FailureFact) ExecutionEvidence {
	return ExecutionEvidence{
		EventID: eventID, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DispatchID: aggregate.ActiveDispatch.ID, AuthoritySnapshotID: aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: operation.ID, Kind: runtimecontract.EvidenceKindStateChange, Status: status, Failure: failure,
		Evidence: runtimecontract.RuntimeOperationEvidence{Quality: common.EvidenceQualityValid},
	}
}

func assertProspectiveCWIState(t *testing.T, aggregate RuntimeAggregate, state runtimecontract.RuntimeCoverageItemState, convergence runtimecontract.RuntimeCWIConvergenceState) {
	t.Helper()
	for _, item := range aggregate.Facts.Coverage {
		if item.ID != "cwi-1" {
			continue
		}
		if item.State != state || item.CWIConvergenceState != convergence {
			t.Fatalf("prospective CWI state = %+v, want state=%s convergence=%s", item, state, convergence)
		}
		return
	}
	t.Fatal("prospective CWI is missing")
}
