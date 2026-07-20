package runtimev2

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestEvaluateCandidateAdmissionClassifiesFullAndPartialReplay(t *testing.T) {
	aggregate, candidate := aggregateWithActiveRejectedCandidate(t)

	full := EvaluateCandidateAdmission(aggregate, candidate)
	if full.Disposition != CandidateAdmissionRejectedContract || full.Failure == nil || full.Failure.Code != candidateAdmissionCodeReplaysRejectedShape {
		t.Fatalf("full replay admission = %+v", full)
	}
	if shapes := ActiveRejectedOperationShapes(aggregate); len(shapes) != 1 || shapes[0].Hash == "" || shapes[0].FailureSignature != "rejected-patch" {
		t.Fatalf("active rejected shapes = %+v", shapes)
	}

	second := candidate.StateChange.Operations[0].Operation
	second.ID = "patch-2"
	content := "corrected\n"
	second.Parameters = patchApplyTestParameters("main.go")
	second.Parameters[2].String = &content
	partialCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		candidate.StateChange.Operations[0], {Operation: second},
	}}}
	partial := EvaluateCandidateAdmission(aggregate, partialCandidate)
	if partial.Disposition != CandidateAdmissionRejectedContract || partial.Failure == nil || partial.Failure.Code != candidateAdmissionCodePartiallyReplaysRejected {
		t.Fatalf("partial replay admission = %+v", partial)
	}

	aggregate.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-repair", Kind: runtimecontract.EvidenceKindStateChange,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000002",
	}}
	if shapes := ActiveRejectedOperationShapes(aggregate); len(shapes) != 0 {
		t.Fatalf("accepted repair must clear active rejected shape: %+v", shapes)
	}
	if cleared := rejectCandidateReplay(aggregate, candidate); !cleared.Accepted() {
		t.Fatalf("accepted repair must clear replay admission: %+v", cleared)
	}
}

func TestActiveRejectedOperationShapesRetainsOnlyCurrentLineageAndInfeasibleProposalFacts(t *testing.T) {
	aggregate, replayedCandidate := aggregateWithActiveRejectedCandidate(t)
	fresh := replayedCandidate.StateChange.Operations[0].Operation
	fresh.ID = "fresh-patch"
	content := "corrected\n"
	fresh.Parameters = patchApplyTestParameters("main.go")
	fresh.Parameters[2].String = &content
	if admitted := EvaluateCandidateAdmission(aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: fresh}}}}); !admitted.Accepted() {
		t.Fatalf("new operation shape rejected: %+v", admitted)
	}

	// An accepted fact before the rejection does not repair the later rejected
	// shape, and evidence outside current focus cannot clear it either.
	aggregate.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "older-accepted", Kind: runtimecontract.EvidenceKindStateChange,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000000",
	}, {
		ID: "other-focus-accepted", Kind: runtimecontract.EvidenceKindStateChange,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-other"}, Revision: "evidence-v2-00000000000000000002",
	}}
	if shapes := ActiveRejectedOperationShapes(aggregate); len(shapes) != 1 {
		t.Fatalf("older or unbound accepted evidence cleared current rejected shape: %+v", shapes)
	}

	infeasible := stateChangeCandidate()
	operation := infeasible.StateChange.Operations[0].Operation
	aggregate = stateChangeAggregate(t)
	transaction := aggregate.Decision.AuthorityTransaction()
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
		Transaction:         transaction, Attempt: 0, StablePrefixHash: "stable-prefix", State: runtimecontract.RuntimeCandidateAttemptStateFailed,
		Proposal: &infeasible,
		Failure:  &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassInfeasible, Code: "operation_target_invalid", Reason: "target invalid"},
		Feasibilities: []runtimecontract.RuntimeCandidateFeasibilityRecord{{
			OperationID: operation.ID, AdmissibilitySnapshotID: "inventory-1", Feasible: false,
			Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_invalid", Signature: "target-invalid", Message: "target invalid"},
		}},
	}}
	shapes := ActiveRejectedOperationShapes(aggregate)
	if len(shapes) != 1 || shapes[0].Hash != runtimecontract.StableRuntimeOperationShape(operation) || shapes[0].FailureCode != "operation_target_invalid" {
		t.Fatalf("failed infeasible proposal did not remain active candidate context: %+v", shapes)
	}
}

func TestEvaluateCandidateAdmissionRespectsRepairDirectiveForValidationGap(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state", Kind: runtimecontract.EvidenceKindStateChange,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000001",
	}}
	aggregate.Facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-state", Kind: runtimecontract.EvidenceKindStateChange,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000002",
	}}
	candidate := stateChangeCandidate()
	rejected := EvaluateCandidateAdmission(aggregate, candidate)
	if rejected.Disposition != CandidateAdmissionRejectedContract || rejected.Failure == nil || rejected.Failure.Code != candidateAdmissionCodeActionEvidenceMismatch {
		t.Fatalf("missing-validation admission = %+v", rejected)
	}
	aggregate.Facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "directive-1", FocusID: aggregate.Facts.Focus.ID,
		RequiredAction: runtimecontract.RuntimeActionProduceStateChange,
	}
	if admitted := EvaluateCandidateAdmission(aggregate, candidate); !admitted.Accepted() {
		t.Fatalf("repair directive must preserve state-change admission: %+v", admitted)
	}
}

func TestAdmitCandidateProposalAtomicallyClosesReplayAttemptAndExhaustsSecondAttempt(t *testing.T) {
	aggregate, candidate := aggregateWithActiveRejectedCandidate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	first, err := controller.AdmitCandidateProposal(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if first.Admission.Failure == nil || first.Admission.Failure.Code != candidateAdmissionCodeReplaysRejectedShape || len(first.Aggregate.CandidateAttempts) != 1 || first.Aggregate.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed {
		t.Fatalf("first rejected proposal = %+v", first)
	}
	if got, want := first.Aggregate.StateRevision, aggregate.StateRevision+2; got != want {
		t.Fatalf("attempt begin + rejection revision = %d, want %d", got, want)
	}
	savesAfterFirst := repository.saves
	duplicate, err := controller.AdmitCandidateProposal(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || duplicate.Admission.Failure == nil || duplicate.Admission.Failure.Code != candidateAdmissionCodeReplaysRejectedShape ||
		duplicate.Aggregate.StateRevision != first.Aggregate.StateRevision || repository.saves != savesAfterFirst {
		t.Fatalf("duplicate rejected proposal must be idempotent: result=%+v err=%v saves=%d", duplicate, err, repository.saves)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	second, err := controller.AdmitCandidateProposal(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if second.Admission.Failure == nil || second.Aggregate.Decision.Action != runtimecontract.RuntimeActionFail || second.Aggregate.Decision.Terminal == nil ||
		second.Aggregate.Decision.Terminal.Code != "candidate_retry_exhausted" || hasPendingCandidateAttempt(second.Aggregate.CandidateAttempts) {
		t.Fatalf("second rejected proposal did not atomically exhaust: %+v", second.Aggregate)
	}
	savesAfterExhaustion := repository.saves
	duplicateExhaustion, err := controller.AdmitCandidateProposal(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, candidate)
	if err != nil || duplicateExhaustion.Admission.Failure == nil || duplicateExhaustion.Admission.Failure.Code != candidateAdmissionCodeReplaysRejectedShape ||
		duplicateExhaustion.Aggregate.StateRevision != second.Aggregate.StateRevision || repository.saves != savesAfterExhaustion {
		t.Fatalf("terminal duplicate rejection must be idempotent: result=%+v err=%v saves=%d", duplicateExhaustion, err, repository.saves)
	}
}

func TestAdmitCandidateProposalIdentifiesInvalidDuplicateByCandidateStructure(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	invalid := runtimecontract.RuntimeCandidate{}
	first, err := controller.AdmitCandidateProposal(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, invalid)
	if err != nil || first.Admission.Failure == nil || first.Admission.Failure.Code != candidateAdmissionCodeInvalid || first.Admission.Failure.Signature != candidateAdmissionInvalidSignature(invalid) {
		t.Fatalf("invalid candidate admission = %+v err=%v", first, err)
	}
	saves := repository.saves
	duplicate, err := controller.AdmitCandidateProposal(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, invalid)
	if err != nil || duplicate.Admission.Failure == nil || duplicate.Admission.Failure.Signature != first.Admission.Failure.Signature || repository.saves != saves {
		t.Fatalf("same invalid candidate was not idempotent: result=%+v err=%v saves=%d", duplicate, err, repository.saves)
	}
	differentInvalid := runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
		Kind: string(runtimecontract.GovernanceDecision), SubjectRef: "different", Reason: "wrong action",
	}}}
	if _, err := controller.AdmitCandidateProposal(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, differentInvalid); err == nil || repository.saves != saves {
		t.Fatalf("different invalid candidate was treated as duplicate: err=%v saves=%d", err, repository.saves)
	}
}

func TestAdmitAndSealCandidateAttemptKeepsRejectedAndAcceptedDuplicateDeliveriesIdempotent(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		aggregate, candidate := aggregateWithActiveRejectedCandidate(t)
		repository := &controllerRepository{aggregate: &aggregate}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		first, err := controller.AdmitAndSealCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
		if err != nil || first.Admission.Failure == nil || first.Aggregate.ActiveDispatch != nil {
			t.Fatalf("first final rejection = %+v err=%v", first, err)
		}
		saves := repository.saves
		duplicate, err := controller.AdmitAndSealCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
		if err != nil || duplicate.Admission.Failure == nil || duplicate.Aggregate.StateRevision != first.Aggregate.StateRevision || repository.saves != saves {
			t.Fatalf("duplicate final rejection = %+v err=%v saves=%d", duplicate, err, repository.saves)
		}
	})

	t.Run("accepted", func(t *testing.T) {
		aggregate := stateChangeAggregate(t)
		repository := &controllerRepository{aggregate: &aggregate}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		candidate := stateChangeCandidate()
		if _, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		first, err := controller.AdmitAndSealCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
		if err != nil || !first.Admission.Accepted() || first.Aggregate.ActiveDispatch == nil {
			t.Fatalf("first seal = %+v err=%v", first, err)
		}
		saves := repository.saves
		duplicate, err := controller.AdmitAndSealCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
		if err != nil || !duplicate.Admission.Accepted() || duplicate.Aggregate.ActiveDispatch == nil || duplicate.Aggregate.StateRevision != first.Aggregate.StateRevision || repository.saves != saves {
			t.Fatalf("duplicate seal = %+v err=%v saves=%d", duplicate, err, repository.saves)
		}
	})
}

func aggregateWithActiveRejectedCandidate(t *testing.T) (RuntimeAggregate, runtimecontract.RuntimeCandidate) {
	t.Helper()
	aggregate := stateChangeAggregate(t)
	candidate := stateChangeCandidate()
	dispatch, err := SealDispatchWithActivation(aggregate.Decision, candidate, aggregate.Facts.Activation)
	if err != nil {
		t.Fatal(err)
	}
	dispatch.State = DispatchStateCommitted
	aggregate.CompletedDispatches = []SealedDispatch{dispatch}
	aggregate.Facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-patch", Kind: runtimecontract.EvidenceKindStateChange,
		DispatchID: dispatch.ID, OperationID: "patch-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: "evidence-v2-00000000000000000001", FailureSignature: "rejected-patch",
	}}
	return aggregate, candidate
}
