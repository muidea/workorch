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

func TestEvaluateCandidateAdmissionRejectsAcceptedNoProgressShape(t *testing.T) {
	aggregate := validationAggregate(t)
	candidate := validationCandidate()
	operation := candidate.Validation.Operations[0].Operation
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID,
		NoProgressOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{
			Operation: operation, FailureCode: "accepted_validation_pending_assessment",
		}},
	}
	result := EvaluateCandidateAdmission(aggregate, candidate)
	if result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeReplaysNoProgressShape {
		t.Fatalf("accepted no-progress replay admission = %+v", result)
	}
	changed := operation
	changed.ID = "validate-other"
	command := "true --other"
	changed.Parameters = append([]runtimecontract.OperationParameter(nil), changed.Parameters...)
	for idx := range changed.Parameters {
		if changed.Parameters[idx].Name == "command" {
			changed.Parameters[idx].String = &command
		}
	}
	if admitted := EvaluateCandidateAdmission(aggregate, runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: changed}}}}); !admitted.Accepted() {
		t.Fatalf("new validation structure rejected: %+v", admitted)
	}
}

func TestRepairableAssessmentSameGapRecordsAcceptedOperationShapeForAdmission(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	candidate := stateChangeCandidate()
	operation := candidate.StateChange.Operations[0].Operation
	aggregate.CompletedDispatches = []SealedDispatch{{
		ID: "dispatch-accepted-state", Candidate: candidate,
	}}
	aggregate.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state", Kind: runtimecontract.EvidenceKindStateChange,
		DispatchID: "dispatch-accepted-state", OperationID: operation.ID,
		CoverageWorkItemIDs: aggregate.Facts.Focus.CoverageWorkItemIDs,
	}}
	ledger := runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID,
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs: aggregate.Facts.Focus.CoverageWorkItemIDs,
			Reason:              "same assessment gap",
		},
	}
	ledger = recordAcceptedNoProgressShapes(aggregate, ledger, "same-gap", ledger.ActiveGap)
	if len(ledger.NoProgressOperationShapes) != 1 || ledger.NoProgressOperationShapes[0].FailureCode != "accepted_operation_same_gap" {
		t.Fatalf("same-gap assessment did not retain accepted state shape: %+v", ledger)
	}
	aggregate.Facts.Repair = ledger
	if result := EvaluateCandidateAdmission(aggregate, candidate); result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeReplaysNoProgressShape {
		t.Fatalf("same-gap accepted state replay admission = %+v", result)
	}
}

func TestEvaluateCandidateAdmissionRejectsDurableChangeOutsideGroundedRepairTarget(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "validation-failure", TargetRefs: []string{"implementation.go"}}

	wrong := stateChangeCandidate()
	result := EvaluateCandidateAdmission(aggregate, wrong)
	if result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding {
		t.Fatalf("outside-target candidate admission = %+v", result)
	}

	correct := stateChangeCandidate()
	correct.StateChange.Operations[0].Operation.TargetRefs = []string{"implementation.go"}
	correct.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters("implementation.go")
	if admitted := EvaluateCandidateAdmission(aggregate, correct); !admitted.Accepted() {
		t.Fatalf("grounded candidate rejected: %+v", admitted)
	}
}

func TestEvaluateCandidateAdmissionAllowsOwnerGroundedWorkspaceScopeThroughFileAdmissibility(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs: []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection

	candidate := stateChangeCandidate()
	newTarget := "new-deliverable.go"
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{newTarget}
	candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters(newTarget)
	if admitted := EvaluateCandidateAdmission(aggregate, candidate); !admitted.Accepted() {
		t.Fatalf("owner-grounded workspace scope candidate rejected before File admissibility: %+v", admitted)
	}
}

func TestEvaluateCandidateAdmissionWorkspaceScopeDoesNotAuthorizeOtherDurableOwners(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs: []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection

	operation := runtimecontract.RuntimeOperation{ID: "initialize-repository", Kind: runtimecontract.StepKindGitWrite, TargetRefs: []string{"implementation.go"}}
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: operation}}}}); result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding {
		t.Fatalf("non-File workspace-scope candidate admission = %+v", result)
	}
}

func TestEvaluateCandidateAdmissionCreationScopeRequiresConcreteDurableFileTarget(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-create", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionCreate, CandidateCount: 0,
		ObservedRevision: "workspace-tree-empty",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection

	gitInit := "init"
	nonFileCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "initialize-repository", Kind: runtimecontract.StepKindGitWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &gitInit}},
	}}}}}
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, nonFileCandidate); result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeCreationScopeMissingTarget {
		t.Fatalf("non-File creation-scope candidate admission = %+v", result)
	}

	fileCandidate := stateChangeCandidate()
	newTarget := "new-deliverable.txt"
	fileCandidate.StateChange.Operations[0].Operation.TargetRefs = []string{newTarget}
	fileCandidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters(newTarget)
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, fileCandidate); !result.Accepted() {
		t.Fatalf("concrete File creation candidate rejected before File owner feasibility: %+v", result)
	}
}

func TestEvaluateCandidateAdmissionScopeCannotBypassOwnerFailureTarget(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "implementation", SelectorRef: "file/workspace_deliverable",
		Disposition:         runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs:          []string{"implementation.go", "support.go"},
		CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
		Signature: "owner-observed-failure", TargetRefs: []string{"implementation.go"},
	}

	candidate := stateChangeCandidate()
	wrongTarget := "support.go"
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{wrongTarget}
	candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters(wrongTarget)
	result := EvaluateCandidateAdmission(aggregate, candidate)
	if result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding {
		t.Fatalf("workspace scope bypassed owner failure target: %+v", result)
	}

	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{"implementation.go"}
	candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters("implementation.go")
	if result := EvaluateCandidateAdmission(aggregate, candidate); !result.Accepted() {
		t.Fatalf("candidate targeting owner failure target rejected: %+v", result)
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
