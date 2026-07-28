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
	second.Parameters[1].String = &content
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

func TestRepairClosureAdmissionRejectsOperationOutsideSealedRepairTarget(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	closure := runtimecontract.RuntimeRepairClosure{
		GapSignature:                      "assessment-gap-v1",
		BaselineEvidenceRevision:          aggregate.Facts.EvidenceRevision,
		CoverageWorkItemIDs:               aggregate.Facts.Focus.CoverageWorkItemIDs,
		MissingEvidenceKinds:              []string{runtimecontract.EvidenceKindValidation},
		GroundedTargetRefs:                []string{"main.go"},
		RequireValidationAfterStateChange: true,
	}
	closure.ID = runtimecontract.StableRuntimeRepairClosureID(closure)
	aggregate.Facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-1", FocusID: aggregate.Facts.Focus.ID,
		Trigger: runtimecontract.RuntimeRepairTriggerAssessmentGap, RequiredAction: runtimecontract.RuntimeActionProduceStateChange,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
		CoverageWorkItemIDs:   aggregate.Facts.Focus.CoverageWorkItemIDs, TargetRefs: []string{"main.go"},
		Closure: &closure,
	}
	aggregate.Decision.RequiredOutput.RepairClosureID = closure.ID
	candidate := stateChangeCandidate()
	candidate.StateChange.Operations[0].Operation = patchApplyTestOperation("repair-other", "other.go")
	result := rejectCandidateRepairClosure(aggregate, candidate)
	if result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeRepairClosureMismatch {
		t.Fatalf("outside repair closure admission = %+v", result)
	}
}

func TestActiveRejectedOperationShapesRetainsOnlyCurrentLineageAndInfeasibleProposalFacts(t *testing.T) {
	aggregate, replayedCandidate := aggregateWithActiveRejectedCandidate(t)
	fresh := replayedCandidate.StateChange.Operations[0].Operation
	fresh.ID = "fresh-patch"
	content := "corrected\n"
	fresh.Parameters = patchApplyTestParameters("main.go")
	fresh.Parameters[1].String = &content
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

func TestOwnerRetargetKeepsOldRejectedShapeDiagnosticButAdmitsNewTargetShape(t *testing.T) {
	aggregate, oldCandidate := aggregateWithActiveRejectedCandidate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
		Signature: "owner-retarget", TargetRefs: []string{"implementation.go"},
		TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
			OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation",
			ObservedRevision: "mcp-catalog-v2", TargetRefs: []string{"implementation.go"},
		},
	}
	aggregate.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "implementation.go", ContentHash: "implementation-v2", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	updatedFacts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts = updatedFacts
	aggregate.Decision = decision
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange || !sameRuntimeTargetRefs(decision.RequiredOutput.TargetRefs, []string{"implementation.go"}) {
		t.Fatalf("retarget decision = %+v", decision)
	}
	shapes := ActiveRejectedOperationShapes(aggregate)
	if len(shapes) != 1 || !sameRuntimeTargetRefs(shapes[0].Operation.TargetRefs, []string{"main.go"}) {
		t.Fatalf("old rejected shape was not retained as diagnostic lineage: %+v", shapes)
	}
	newCandidate := oldCandidate
	newCandidate.StateChange.Operations[0].Operation = patchApplyTestOperation("patch-implementation", "implementation.go")
	if runtimecontract.StableRuntimeOperationShape(newCandidate.StateChange.Operations[0].Operation) == shapes[0].Hash {
		t.Fatal("retarget must produce a distinct canonical operation shape")
	}
	if admission := EvaluateCandidateAdmission(aggregate, newCandidate); !admission.Accepted() {
		t.Fatalf("owner-retargeted operation shape was rejected: %+v", admission)
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

func TestEvaluateCandidateAdmissionUsesSemanticContractNotOperationIdentityOrParameterOrder(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	base := stateChangeCandidate()

	equivalent := base
	equivalent.StateChange.Operations = append([]runtimecontract.StateChangingOperation(nil), base.StateChange.Operations...)
	equivalent.StateChange.Operations[0].Operation.ID = "provider-chosen-operation-id"
	parameters := append([]runtimecontract.OperationParameter(nil), base.StateChange.Operations[0].Operation.Parameters...)
	for left, right := 0, len(parameters)-1; left < right; left, right = left+1, right-1 {
		parameters[left], parameters[right] = parameters[right], parameters[left]
	}
	equivalent.StateChange.Operations[0].Operation.Parameters = parameters
	if admitted := EvaluateCandidateAdmission(aggregate, equivalent); !admitted.Accepted() {
		t.Fatalf("semantic-equivalent candidate was rejected: %+v", admitted)
	}

	for _, testCase := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeOperation)
	}{
		{
			name: "kind outside sealed allowed set",
			mutate: func(operation *runtimecontract.RuntimeOperation) {
				command := "true"
				operation.Kind = runtimecontract.StepKindShellCommand
				operation.TargetRefs = nil
				operation.Parameters = []runtimecontract.OperationParameter{{Name: "command", String: &command}}
			},
		},
		{
			name: "target ref disagrees with operation target parameter",
			mutate: func(operation *runtimecontract.RuntimeOperation) {
				operation.TargetRefs = []string{"other.go"}
			},
		},
		{
			name: "coverage outside sealed coverage",
			mutate: func(operation *runtimecontract.RuntimeOperation) {
				operation.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-other"}
			},
		},
		{
			name: "dependency outside candidate graph",
			mutate: func(operation *runtimecontract.RuntimeOperation) {
				operation.DependsOn = []string{"missing-operation"}
			},
		},
		{
			name: "parameter outside sealed operation contract",
			mutate: func(operation *runtimecontract.RuntimeOperation) {
				value := "unsupported"
				operation.Parameters = append(operation.Parameters, runtimecontract.OperationParameter{Name: "notAllowed", String: &value})
			},
		},
		{
			name: "variant outside sealed operation contract",
			mutate: func(operation *runtimecontract.RuntimeOperation) {
				operation.Parameters = append([]runtimecontract.OperationParameter(nil), operation.Parameters...)
				for index := range operation.Parameters {
					if operation.Parameters[index].Name == "patchType" {
						value := "not-a-patch-variant"
						operation.Parameters[index].String = &value
					}
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := stateChangeCandidate()
			operation := candidate.StateChange.Operations[0].Operation
			testCase.mutate(&operation)
			candidate.StateChange.Operations[0].Operation = operation
			result := EvaluateCandidateAdmission(aggregate, candidate)
			if result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Diagnostic == nil || result.Failure.Diagnostic.Path == "" ||
				result.Failure.Class != runtimecontract.RuntimeCandidateFailureClassContract ||
				result.Failure.Code != candidateAdmissionCodeInvalid || result.Failure.Signature == "" || result.Failure.Reason == "" ||
				result.Correction == nil || result.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || len(result.Correction.Objectives) == 0 {
				t.Fatalf("semantic contract admission = %+v", result)
			}
		})
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
	// After N1/N2 sealed TargetPolicy, a FailureTarget-backed decision compiles
	// exact_target membership. Outside targets are rejected as contract-invalid
	// first-failure; SM-3 grounding remains the fallback when membership is not sealed.
	if result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil ||
		(result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding && result.Failure.Code != candidateAdmissionCodeInvalid) {
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
	aggregate.Facts.Focus.TargetSelectorRef = "file/workspace_deliverable"
	aggregate.Facts.Focus.ProspectiveTargetRef = "new-deliverable.txt"
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs: []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	bindCurrentFocusTargetSelection(&aggregate.Facts)

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
	bindCurrentFocusTargetSelection(&aggregate.Facts)

	operation := runtimecontract.RuntimeOperation{ID: "initialize-repository", Kind: runtimecontract.StepKindGitWrite, TargetRefs: []string{"implementation.go"}}
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: operation}}}}); result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding {
		t.Fatalf("non-File workspace-scope candidate admission = %+v", result)
	}
}

func TestEvaluateCandidateAdmissionWorkspaceScopeAllowsExplicitFileGroundedTechnicalOperation(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs: []string{"go.mod", "go.sum"}, CandidateTargetRefs: []string{"go.mod", "go.sum"}, CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	bindCurrentFocusTargetSelection(&aggregate.Facts)

	shell := runtimecontract.RuntimeOperation{ID: "tidy-modules", Kind: runtimecontract.StepKindShellCommand, TargetRefs: []string{"go.sum"}}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: shell}}}}
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, candidate); !result.Accepted() {
		t.Fatalf("File-grounded shell operation rejected: %+v", result)
	}
}

func TestEvaluateCandidateAdmissionWorkspaceScopeRejectsTechnicalOperationOutsideFileInventoryOrMixedOwnerBypass(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs: []string{"go.mod", "go.sum"}, CandidateTargetRefs: []string{"go.mod", "go.sum"}, CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	bindCurrentFocusTargetSelection(&aggregate.Facts)

	tests := []struct {
		name      string
		candidate runtimecontract.RuntimeCandidate
	}{
		{
			name: "shell outside inventory",
			candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
				Operation: runtimecontract.RuntimeOperation{ID: "tidy-outside", Kind: runtimecontract.StepKindShellCommand, TargetRefs: []string{"outside.go"}},
			}}}},
		},
		{
			name: "shell missing target",
			candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
				Operation: runtimecontract.RuntimeOperation{ID: "tidy-no-target", Kind: runtimecontract.StepKindShellCommand},
			}}}},
		},
		{
			name: "mixed file and git durable operations",
			candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
				{Operation: patchApplyTestOperation("write-go-sum", "go.sum")},
				{Operation: runtimecontract.RuntimeOperation{ID: "git-bypass", Kind: runtimecontract.StepKindGitWrite, TargetRefs: []string{"go.sum"}}},
			}}},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, testCase.candidate)
			if result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding {
				t.Fatalf("workspace scope bypass result = %+v", result)
			}
		})
	}
}

func TestEvaluateCandidateAdmissionCreationScopeRequiresConcreteDurableFileTarget(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.TargetSelectorRef = "file/workspace_deliverable"
	aggregate.Facts.Focus.ProspectiveTargetRef = "new-deliverable.txt"
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-create", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: "new-deliverable.txt", CandidateCount: 0,
		ObservedRevision: "workspace-tree-empty",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	bindCurrentFocusTargetSelection(&aggregate.Facts)

	gitInit := "init"
	nonFileCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "initialize-repository", Kind: runtimecontract.StepKindGitWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &gitInit}},
	}}}}}
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, nonFileCandidate); result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding {
		t.Fatalf("non-File creation-scope candidate admission = %+v", result)
	}

	fileCandidate := stateChangeCandidate()
	newTarget := "new-deliverable.txt"
	fileCandidate.StateChange.Operations[0].Operation.TargetRefs = []string{newTarget}
	fileCandidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters(newTarget)
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, fileCandidate); !result.Accepted() {
		t.Fatalf("concrete File creation candidate rejected before File owner feasibility: %+v", result)
	}
	shellCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "shell-create-bypass", Kind: runtimecontract.StepKindShellCommand, TargetRefs: []string{newTarget}},
	}}}}
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, shellCandidate); result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding {
		t.Fatalf("shell creation-scope candidate admission = %+v", result)
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
	bindCurrentFocusTargetSelection(&aggregate.Facts)
	aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
		Signature: "owner-observed-failure", TargetRefs: []string{"implementation.go"},
	}

	candidate := stateChangeCandidate()
	wrongTarget := "support.go"
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{wrongTarget}
	candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters(wrongTarget)
	result := EvaluateCandidateAdmission(aggregate, candidate)
	// FailureTarget + scope still seals exclusive exact membership for the
	// grounded repair target. A scope inventory member that is not the sealed
	// failure target is rejected before SM-3 grounding as contract-invalid, or
	// by SM-3 grounding when membership is not exclusive.
	if result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil ||
		(result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding && result.Failure.Code != candidateAdmissionCodeInvalid) {
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
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	first, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if first.Admission.Failure == nil || first.Admission.Failure.Code != candidateAdmissionCodeReplaysRejectedShape || len(first.Aggregate.CandidateAttempts) != 1 || first.Aggregate.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed {
		t.Fatalf("first rejected proposal = %+v", first)
	}
	if got, want := first.Aggregate.StateRevision, aggregate.StateRevision+5; got != want {
		t.Fatalf("complete session + attempt + rejection revision = %d, want %d", got, want)
	}
	savesAfterFirst := repository.saves
	duplicate, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || duplicate.Admission.Failure == nil || duplicate.Admission.Failure.Code != candidateAdmissionCodeReplaysRejectedShape ||
		duplicate.Aggregate.StateRevision != first.Aggregate.StateRevision || repository.saves != savesAfterFirst {
		t.Fatalf("duplicate rejected proposal must be idempotent: result=%+v err=%v saves=%d", duplicate, err, repository.saves)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	second, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if second.Admission.Failure == nil || second.Aggregate.Decision.Action != runtimecontract.RuntimeActionFail || second.Aggregate.Decision.Terminal == nil ||
		second.Aggregate.Decision.Terminal.Code != "candidate_retry_exhausted" || hasPendingCandidateAttempt(second.Aggregate.CandidateAttempts) {
		t.Fatalf("second rejected proposal did not atomically exhaust: %+v", second.Aggregate)
	}
	savesAfterExhaustion := repository.saves
	duplicateExhaustion, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, candidate)
	if err != nil || duplicateExhaustion.Admission.Failure == nil || duplicateExhaustion.Admission.Failure.Code != candidateAdmissionCodeReplaysRejectedShape ||
		duplicateExhaustion.Aggregate.StateRevision != second.Aggregate.StateRevision || repository.saves != savesAfterExhaustion {
		t.Fatalf("terminal duplicate rejection must be idempotent: result=%+v err=%v saves=%d", duplicateExhaustion, err, repository.saves)
	}
}

func TestAdmitCandidateProposalExhaustsSemanticReplayWithoutOperationIdentity(t *testing.T) {
	newGroundedAggregate := func() RuntimeAggregate {
		aggregate := stateChangeAggregate(t)
		aggregate.Facts.Focus.RequireTargetGrounding = true
		aggregate.Facts.Focus.TargetRole = "implementation"
		aggregate.Facts.Focus.TargetRefs = nil
		aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
			Signature: "owner-observed-failure", TargetRefs: []string{"implementation.go"},
		}
		// Keep Decision aligned with Facts so the sealed TargetPolicy matches
		// the FailureTarget membership the production reducer would compile.
		decision, err := ReduceRuntimeState(aggregate.Facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		aggregate.Decision = decision
		return aggregate
	}
	wrongCandidate := stateChangeCandidate()
	equivalent := wrongCandidate
	equivalent.StateChange.Operations = append([]runtimecontract.StateChangingOperation(nil), wrongCandidate.StateChange.Operations...)
	equivalent.StateChange.Operations[0].Operation.ID = "provider-rewritten-id"
	parameters := append([]runtimecontract.OperationParameter(nil), equivalent.StateChange.Operations[0].Operation.Parameters...)
	for left, right := 0, len(parameters)-1; left < right; left, right = left+1, right-1 {
		parameters[left], parameters[right] = parameters[right], parameters[left]
	}
	equivalent.StateChange.Operations[0].Operation.Parameters = parameters

	t.Run("semantic replay exhausts exactly once", func(t *testing.T) {
		aggregate := newGroundedAggregate()
		repository := &controllerRepository{aggregate: &aggregate}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		first, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, wrongCandidate)
		// Sealed exact_target membership rejects main.go as contract-invalid; SM-3
		// grounding remains valid when membership is not exclusive.
		if err != nil || first.Admission.Failure == nil ||
			(first.Admission.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding && first.Admission.Failure.Code != candidateAdmissionCodeInvalid) ||
			first.Aggregate.Decision.Action == runtimecontract.RuntimeActionFail {
			t.Fatalf("first grounded-target rejection = %+v err=%v", first, err)
		}
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		second, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, equivalent)
		// Attempt 1 of the same out-of-membership shape still exhausts. The first
		// failure may be sealed exact_target membership (contract-invalid) or SM-3
		// grounding; attempt 1 may surface semantic replay of that rejected shape
		// or the same contract-invalid first-failure again.
		if err != nil || second.Admission.Failure == nil ||
			(second.Admission.Failure.Code != candidateAdmissionCodeReplaysRejectedCandidate &&
				second.Admission.Failure.Code != candidateAdmissionCodeInvalid &&
				second.Admission.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding) ||
			second.Aggregate.Decision.Action != runtimecontract.RuntimeActionFail || second.Aggregate.Decision.Terminal == nil || second.Aggregate.Decision.Terminal.Code != "candidate_retry_exhausted" ||
			hasPendingCandidateAttempt(second.Aggregate.CandidateAttempts) || second.Aggregate.ActiveDispatch != nil || len(second.Aggregate.GovernanceRequests) != 0 {
			t.Fatalf("semantic replay did not atomically exhaust without dispatch/governance: %+v err=%v", second, err)
		}
		saves := repository.saves
		duplicate, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, equivalent)
		if err != nil || duplicate.Admission.Failure == nil ||
			(duplicate.Admission.Failure.Code != candidateAdmissionCodeReplaysRejectedCandidate &&
				duplicate.Admission.Failure.Code != candidateAdmissionCodeInvalid &&
				duplicate.Admission.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding) ||
			duplicate.Aggregate.StateRevision != second.Aggregate.StateRevision || repository.saves != saves {
			t.Fatalf("semantic terminal duplicate was not idempotent: result=%+v err=%v saves=%d", duplicate, err, repository.saves)
		}
	})

	t.Run("retarget remains admissible", func(t *testing.T) {
		aggregate := newGroundedAggregate()
		repository := &controllerRepository{aggregate: &aggregate}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		if _, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, wrongCandidate); err != nil {
			t.Fatal(err)
		}
		retargeted := stateChangeCandidate()
		retargeted.StateChange.Operations[0].Operation = patchApplyTestOperation("provider-corrected-id", "implementation.go")
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		second, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, retargeted)
		if err != nil || !second.Admission.Accepted() || second.Aggregate.Decision.Action == runtimecontract.RuntimeActionFail || hasPendingCandidateAttempt(second.Aggregate.CandidateAttempts) == false {
			t.Fatalf("legal retarget was incorrectly classified as semantic replay: %+v err=%v", second, err)
		}
	})
}

func TestAdmitCandidateProposalIdentifiesInvalidDuplicateByCandidateStructure(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	invalid := runtimecontract.RuntimeCandidate{}
	first, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, invalid)
	if err != nil || first.Admission.Failure == nil || first.Admission.Failure.Code != candidateAdmissionCodeInvalid || first.Admission.Failure.Signature != candidateAdmissionInvalidSignature(invalid) {
		t.Fatalf("invalid candidate admission = %+v err=%v", first, err)
	}
	saves := repository.saves
	duplicate, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, invalid)
	if err != nil || duplicate.Admission.Failure == nil || duplicate.Admission.Failure.Signature != first.Admission.Failure.Signature || repository.saves != saves {
		t.Fatalf("same invalid candidate was not idempotent: result=%+v err=%v saves=%d", duplicate, err, repository.saves)
	}
	differentInvalid := runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
		Kind: string(runtimecontract.GovernanceDecision), SubjectRef: "different", Reason: "wrong action",
	}}}
	if _, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, differentInvalid); err == nil || repository.saves != saves {
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
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		first, err := controller.AdmitAndSealCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
		if err != nil || first.Admission.Failure == nil || first.Aggregate.ActiveDispatch != nil {
			t.Fatalf("first final rejection = %+v err=%v", first, err)
		}
		saves := repository.saves
		duplicate, err := controller.AdmitAndSealCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
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
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		first, err := controller.AdmitAndSealCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
		if err != nil || !first.Admission.Accepted() || first.Aggregate.ActiveDispatch == nil {
			t.Fatalf("first seal = %+v err=%v", first, err)
		}
		saves := repository.saves
		duplicate, err := controller.AdmitAndSealCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
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
