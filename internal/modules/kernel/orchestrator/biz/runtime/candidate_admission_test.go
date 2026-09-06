package runtime

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

	metadataOnly := candidate.StateChange.Operations[0].Operation
	metadataOnly.ID = "patch-retry-with-new-id"
	summary := "retry with different wording"
	metadataOnly.Parameters = append(append([]runtimecontract.OperationParameter(nil), metadataOnly.Parameters...), runtimecontract.OperationParameter{Name: "summary", String: &summary})
	metadataReplay := EvaluateCandidateAdmission(aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: metadataOnly}}}})
	if metadataReplay.Disposition != CandidateAdmissionRejectedContract || metadataReplay.Failure == nil || metadataReplay.Failure.Code != candidateAdmissionCodeReplaysRejectedShape {
		t.Fatalf("operation ID/summary-only replay admission = %+v", metadataReplay)
	}

	second := candidate.StateChange.Operations[0].Operation
	second.ID = "patch-2"
	content := "corrected\n"
	second.Parameters = patchApplyTestParameters("main.go")
	second.Parameters[1].String = &content
	partialCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		candidate.StateChange.Operations[0], {Operation: second},
	}}}
	// The complete admission boundary now rejects this same-owner/same-target
	// pair before replay analysis. Exercise the replay detector directly so its
	// partial-set classification remains covered without treating the prohibited
	// operation set as an admissible candidate shape.
	partial := rejectCandidateReplay(aggregate, partialCandidate)
	if partial.Disposition != CandidateAdmissionRejectedContract || partial.Failure == nil || partial.Failure.Code != candidateAdmissionCodePartiallyReplaysRejected {
		t.Fatalf("partial replay admission = %+v", partial)
	}

	aggregate.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-repair", Kind: runtimecontract.EvidenceKindStateChange,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "runtime-evidence-00000000000000000002",
		BindingRefs: []string{aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID},
	}}
	if shapes := ActiveRejectedOperationShapes(aggregate); len(shapes) != 0 {
		t.Fatalf("accepted repair must clear active rejected shape: %+v", shapes)
	}
	if cleared := rejectCandidateReplay(aggregate, candidate); !cleared.Accepted() {
		t.Fatalf("accepted repair must clear replay admission: %+v", cleared)
	}
}

func TestActiveRejectedOperationShapesUsesCompletedDispatchFocusForNonCreditingReceipt(t *testing.T) {
	aggregate, replayedCandidate := aggregateWithActiveRejectedCandidate(t)
	aggregate.Facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "focus-next", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-next"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
	}}
	// A rejected owner receipt is non-crediting by definition. Its immutable
	// dispatch transaction, not a fabricated CWI, binds it to the active focus.
	aggregate.Facts.Evidence.Rejected[0].CoverageWorkItemIDs = nil
	aggregate.Facts.Evidence.Rejected[0].BindingRefs = []string{aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID}

	shapes := ActiveRejectedOperationShapes(aggregate)
	if len(shapes) != 1 || shapes[0].Operation.ID != runtimeTestCandidateOperationID(t, replayedCandidate, 0) {
		t.Fatalf("current-focus non-crediting rejected shape = %+v", shapes)
	}
	replayed := replayedCandidate.StateChange.Operations[0].Operation
	replayed.ID = "patch-retry"
	summary := "same operation with revised wording"
	replayed.Parameters = append(append([]runtimecontract.OperationParameter(nil), replayed.Parameters...), runtimecontract.OperationParameter{Name: "summary", String: &summary})
	if result := rejectCandidateReplay(aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: replayed}}}}); result.Accepted() || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeReplaysRejectedShape {
		t.Fatalf("current-focus replay was not rejected before owner call: %+v", result)
	}

	different := replayedCandidate.StateChange.Operations[0].Operation
	different.ID = "patch-different-content"
	content := "different execution semantics\n"
	different.Parameters = patchApplyTestParameters("main.go")
	different.Parameters[1].String = &content
	if result := rejectCandidateReplay(aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: different}}}}); !result.Accepted() {
		t.Fatalf("different operation shape was rejected: %+v", result)
	}

	otherFocus := aggregate
	otherFocus.CompletedDispatches = append([]SealedDispatch(nil), aggregate.CompletedDispatches...)
	otherFocus.CompletedDispatches[0].Transaction.FocusID = "focus-next"
	if shapes := ActiveRejectedOperationShapes(otherFocus); len(shapes) != 0 {
		t.Fatalf("other-focus dispatch leaked rejected shape into current focus: %+v", shapes)
	}

	newerAccepted := aggregate
	newerAccepted.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-after-rejection", Kind: runtimecontract.EvidenceKindStateChange,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...),
		BindingRefs:         []string{aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID},
		Revision:            "runtime-evidence-00000000000000000002",
	}}
	if shapes := ActiveRejectedOperationShapes(newerAccepted); len(shapes) != 0 {
		t.Fatalf("newer accepted current-focus evidence did not retire rejected shape: %+v", shapes)
	}
}

func TestRepairClosureAdmissionRejectsOperationOutsideSealedRepairTarget(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	closure := runtimecontract.RuntimeRepairClosure{
		GapSignature:                      "assessment-gap-v1",
		BaselineEvidenceRevision:          aggregate.Facts.EvidenceRevision,
		CoverageWorkItemIDs:               aggregate.Facts.Focus.CoverageWorkItemIDs,
		RequiredEvidenceKinds:             []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		GroundedTargetRefs:                []string{"main.go"},
		RequireValidationAfterStateChange: true,
	}
	closure.ID = runtimecontract.StableRuntimeRepairClosureID(closure)
	aggregate.Facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-1", FocusID: aggregate.Facts.Focus.ID,
		Trigger: runtimecontract.RuntimeRepairTriggerAssessmentGap, RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
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

func TestRepairClosureAdmissionPermitsNonCreditingCurrentStateValidation(t *testing.T) {
	facts := controllerFacts()
	closure := runtimecontract.RuntimeRepairClosure{
		GapSignature:             "no-op-provenance-v1",
		BaselineEvidenceRevision: facts.EvidenceRevision,
		CoverageWorkItemIDs:      facts.Focus.CoverageWorkItemIDs,
		RequiredEvidenceKinds:    []string{runtimecontract.EvidenceKindStateChange},
		GroundedTargetRefs:       []string{"target.txt"},
	}
	closure.ID = runtimecontract.StableRuntimeRepairClosureID(closure)
	aggregate := RuntimeAggregate{Facts: facts}
	aggregate.Facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-no-op", FocusID: facts.Focus.ID,
		RequiredState: runtimecontract.ConvergenceStateNeedValidation,
		Closure:       &closure,
	}
	aggregate.Decision = runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedValidation,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			RepairClosureID: closure.ID,
		},
	}

	// The focus has only a durable state-change binding. A file/read operation
	// can therefore validate current state but cannot receive fabricated CWI
	// credit or be forced through the state-change closure binding.
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "inspect-current-state", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{"target.txt"}},
	}}}}
	if result := rejectCandidateRepairClosure(aggregate, candidate); !result.Accepted() {
		t.Fatalf("non-crediting validation rejected by state-change closure: %+v", result)
	}
}

func TestRepairClosureAdmissionNeedsCompletedLineageNotValidationLabels(t *testing.T) {
	aggregate := validationOnlyAggregateWithPreparation(t, nil)
	candidate := runtimeTestMustDeriveCandidateAuthority(t, aggregate, validationCandidate())
	closure := runtimecontract.RuntimeRepairClosure{
		GapSignature: "unconfirmed-repair", BaselineEvidenceRevision: aggregate.Facts.EvidenceRevision,
		CoverageWorkItemIDs:   aggregate.Facts.Focus.CoverageWorkItemIDs,
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		GroundedTargetRefs:    []string{"repair-target"}, RequireValidationAfterStateChange: true,
	}
	closure.ID = runtimecontract.StableRuntimeRepairClosureID(closure)
	aggregate.Decision.RequiredOutput.RepairClosureID = closure.ID
	// Empty purpose/targets and NeedValidation are not proof that the owner
	// completed a repair. An ordinary closure retains its target guard.
	aggregate.Facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "unconfirmed-fresh-validation", FocusID: aggregate.Facts.Focus.ID,
		RequiredState: runtimecontract.ConvergenceStateNeedValidation, Closure: &closure,
	}
	admission := rejectCandidateRepairClosure(aggregate, candidate)
	if admission.Accepted() || admission.Failure.Code != candidateAdmissionCodeRepairClosureMismatch {
		t.Fatalf("validation labels retired repair target authority: %+v", admission)
	}
	// Even the canonical-looking ID must reverse-prove the exact completed
	// transaction, confirmation and receipts before it can retire those targets.
	aggregate.Facts.RepairDirective.ID = "confirmed-repair/" + aggregate.Facts.Focus.ID + "/" + closure.ID + "/fresh-validation"
	admission = rejectCandidateRepairClosure(aggregate, candidate)
	if admission.Accepted() || admission.Failure.Code != "runtime_contract_defect" {
		t.Fatalf("forged canonical handoff acquired validation authority: %+v", admission)
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
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "runtime-evidence-00000000000000000000",
	}, {
		ID: "other-focus-accepted", Kind: runtimecontract.EvidenceKindStateChange,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-other"}, Revision: "runtime-evidence-00000000000000000002",
	}}
	if shapes := ActiveRejectedOperationShapes(aggregate); len(shapes) != 1 {
		t.Fatalf("older or unbound accepted evidence cleared current rejected shape: %+v", shapes)
	}

	infeasible := stateChangeCandidate()
	aggregate = stateChangeAggregate(t)
	ownerFailure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_invalid", Signature: "target-invalid", Message: "target invalid"}
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, runtimecontract.RuntimeFeedbackDomainFeasibility, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryTargetNotRegular), runtimecontract.RuntimeModuleFile, *ownerFailure)
	if err != nil {
		t.Fatal(err)
	}
	ownerObservation, err := runtimecontract.RuntimeOwnerFeasibilityObservationFromOutcome(runtimecontract.RuntimeOwnerFeasibilityInfeasible, &feedback, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
		Transaction:         transaction, Attempt: 0, StablePrefixHash: "stable-prefix", State: runtimecontract.RuntimeCandidateAttemptStateFailed,
		Proposal: &infeasible,
		Failure:  &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Reason: "target invalid"},
		Feasibilities: []runtimecontract.RuntimeCandidateFeasibilityRecord{{
			OperationID: infeasible.StateChange.Operations[0].Operation.ID, AdmissibilitySnapshotID: "inventory-1",
			Observation: &ownerObservation,
		}},
	}}
	if shapes := ActiveRejectedOperationShapes(aggregate); len(shapes) != 0 {
		t.Fatalf("candidate attempt facts must not synthesize rejected operation lineage: %+v", shapes)
	}
}

func TestOwnerRetargetKeepsOldRejectedShapeDiagnosticButAdmitsNewTargetShape(t *testing.T) {
	aggregate, oldCandidate := aggregateWithActiveRejectedCandidateWithPreparation(t, func(initial *RuntimeAggregate) {
		initial.Facts.Focus.RequireTargetGrounding = true
		initial.Facts.Focus.TargetRole = "implementation"
	})
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
	// The owner retarget is a new current repair authority. Preserve the old
	// rejected receipt/fact as diagnostic lineage, but do not retain the repair
	// directive derived from its superseded target observation.
	aggregate.Facts.RepairDirective = nil
	updatedFacts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts = updatedFacts
	aggregate.Decision = decision
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange || !sameRuntimeTargetRefs(decision.RequiredOutput.TargetRefs, []string{"implementation.go"}) {
		t.Fatalf("retarget decision = %+v", decision)
	}
	shapes := ActiveRejectedOperationShapes(aggregate)
	if len(shapes) != 1 || !sameRuntimeTargetRefs(shapes[0].Operation.TargetRefs, []string{"main.go"}) {
		t.Fatalf("old rejected shape was not retained as diagnostic lineage: %+v", shapes)
	}
	newCandidate := oldCandidate
	newCandidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), oldCandidate.StateChange.Operations...)}
	newCandidate.StateChange.Operations[0].Operation.ID = "patch-implementation"
	newCandidate.StateChange.Operations[0].Operation.TargetRefs = []string{"implementation.go"}
	newCandidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters("implementation.go")
	if runtimecontract.StableRuntimeOperationShape(newCandidate.StateChange.Operations[0].Operation) == shapes[0].Hash {
		t.Fatal("retarget must produce a distinct canonical operation shape")
	}
	if admission := EvaluateCandidateAdmission(aggregate, newCandidate); !admission.Accepted() {
		t.Fatalf("owner-retargeted operation shape was rejected: %+v", admission)
	}
}

func TestEvaluateCandidateAdmissionRespectsRepairDirectiveForValidationGap(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	bindingRef := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
	aggregate.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state", Kind: runtimecontract.EvidenceKindStateChange,
		FocusID: aggregate.Facts.Focus.ID, BindingRefs: []string{bindingRef}, Quality: common.EvidenceQualityValid,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "runtime-evidence-00000000000000000001",
	}}
	aggregate.Facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-state", Kind: runtimecontract.EvidenceKindStateChange,
		FocusID: aggregate.Facts.Focus.ID, BindingRefs: []string{bindingRef}, Quality: common.EvidenceQualityInvalid,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "runtime-evidence-00000000000000000002",
	}}
	candidate := runtimeTestBindCandidateToCurrentFocus(t, aggregate, stateChangeCandidate())
	rejected := EvaluateCandidateAdmission(aggregate, candidate)
	if rejected.Disposition != CandidateAdmissionRejectedContract || rejected.Failure == nil || rejected.Failure.Code != candidateAdmissionCodeActionEvidenceMismatch {
		t.Fatalf("missing-validation admission=%+v failure=%+v correction=%+v", rejected, rejected.Failure, rejected.Correction)
	}
	aggregate.Facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "directive-1", FocusID: aggregate.Facts.Focus.ID,
		RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
	}
	if admitted := EvaluateCandidateAdmission(aggregate, candidate); !admitted.Accepted() {
		t.Fatalf("repair directive must preserve state-change admission: %+v", admitted)
	}
}

func TestEvaluateCandidateAdmissionUsesSemanticContractNotOperationIdentityOrParameterOrder(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	base := runtimeTestBindCandidateToCurrentFocus(t, aggregate, stateChangeCandidate())

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
		name        string
		correctable bool
		mutate      func(*runtimecontract.RuntimeOperation)
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
			name: "target ref disagrees with operation target parameter", correctable: true,
			mutate: func(operation *runtimecontract.RuntimeOperation) {
				operation.TargetRefs = []string{"other.go"}
			},
		},
		{
			name: "coverage outside sealed coverage", correctable: true,
			mutate: func(operation *runtimecontract.RuntimeOperation) {
				operation.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-other"}
			},
		},
		{
			name: "parameter outside sealed operation contract", correctable: true,
			mutate: func(operation *runtimecontract.RuntimeOperation) {
				value := "unsupported"
				operation.Parameters = append(operation.Parameters, runtimecontract.OperationParameter{Name: "notAllowed", String: &value})
			},
		},
		{
			name: "variant outside sealed operation contract", correctable: true,
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
			candidate := base
			candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), base.StateChange.Operations...)}
			operation := candidate.StateChange.Operations[0].Operation
			operation.TargetRefs = append([]string(nil), operation.TargetRefs...)
			operation.BindingRefs = append([]string(nil), operation.BindingRefs...)
			operation.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), operation.CoverageWorkItemIDs...)
			operation.Parameters = append([]runtimecontract.OperationParameter(nil), operation.Parameters...)
			testCase.mutate(&operation)
			candidate.StateChange.Operations[0].Operation = operation
			result := EvaluateCandidateAdmission(aggregate, candidate)
			if result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Diagnostic == nil || result.Failure.Diagnostic.Path == "" ||
				result.Failure.Class != runtimecontract.RuntimeCandidateFailureClassContract ||
				result.Failure.Code != candidateAdmissionCodeInvalid || result.Failure.Signature == "" || result.Failure.Reason == "" || result.Correction == nil {
				t.Fatalf("semantic contract admission = %+v failure=%+v correction=%+v", result, result.Failure, result.Correction)
			}
			if testCase.correctable {
				if result.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || len(result.Correction.Objectives) == 0 || result.Correction.Witness == nil {
					t.Fatalf("semantic contract correction has no concrete witness: %+v", result)
				}
			} else if result.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || result.Correction.TerminalCode != "candidate_authority_exhausted" {
				t.Fatalf("non-total semantic correction did not exhaust authority: %+v", result)
			}
		})
	}

	t.Run("dependency authority is manifest derived", func(t *testing.T) {
		candidate := stateChangeCandidate()
		candidate.StateChange.Operations[0].Operation.DependsOn = []string{"provider-authored-dependency"}
		result := EvaluateCandidateAdmission(aggregate, candidate)
		if !result.Accepted() {
			t.Fatalf("manifest-derived dependency authority was not admitted: %+v", result)
		}
	})
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
	changed.Parameters = append([]runtimecontract.OperationParameter(nil), changed.Parameters...)
	for idx := range changed.Parameters {
		if changed.Parameters[idx].Name == "recipeId" {
			recipeID := "test-managed-validation-alternative"
			changed.Parameters[idx].String = &recipeID
		}
	}
	if admitted := EvaluateCandidateAdmission(aggregate, runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: changed}}}}); admitted.Accepted() || admitted.Failure == nil || admitted.Failure.Code != candidateAdmissionCodeInvalid {
		t.Fatalf("validation recipe outside the sealed CWI binding was accepted: %+v failure=%+v correction=%+v", admitted, admitted.Failure, admitted.Correction)
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
		BindingRefs:         []string{aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID},
	}}
	ledger := runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID,
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs: aggregate.Facts.Focus.CoverageWorkItemIDs,
			Reason:              "same assessment gap",
		},
	}
	ledger = recordAcceptedNoProgressShapes(aggregate, ledger, "same-gap")
	if len(ledger.NoProgressOperationShapes) != 1 || ledger.NoProgressOperationShapes[0].FailureCode != "accepted_operation_same_gap" {
		t.Fatalf("same-gap assessment did not retain accepted state shape: %+v", ledger)
	}
	aggregate.Facts.Repair = ledger
	if result := EvaluateCandidateAdmission(aggregate, candidate); result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeReplaysNoProgressShape {
		t.Fatalf("same-gap accepted state replay admission = %+v", result)
	}
}

func TestEvaluateCandidateAdmissionRejectsDurableChangeOutsideGroundedRepairTarget(t *testing.T) {
	aggregate := stateChangeAggregateWithPreparation(t, func(initial *RuntimeAggregate) {
		initial.Facts.Focus.RequireTargetGrounding = true
		initial.Facts.Focus.TargetRole = "implementation"
		initial.Facts.Focus.TargetRefs = []string{"implementation.go"}
	})
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

func TestEvaluateCandidateAdmissionAllowsFiniteOwnerGroundedWorkspaceScopeThroughFileAdmissibility(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.TargetSelectorRef = "file/workspace_deliverable"
	aggregate.Facts.Focus.ProspectiveTargetRefs = []string{"new-deliverable.txt"}
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs: []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	runtimeTestSealCurrentFocusBindingTargetSurface(&aggregate.Facts, runtimeTestBindingTargetSurface{
		SelectorRef: selection.SelectorRef, SelectorOwnerRef: selection.OwnerModuleID, SelectorSubjectRef: selection.TargetRole,
	})
	bindCurrentFocusTargetSelection(&aggregate.Facts)
	addCandidateAdmissionTargetPolicyFixture(&aggregate.Facts, runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeBlockKindPatchApply, "file_write", runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, selection.TargetRefs)

	candidate := stateChangeCandidate()
	groundedTarget := "implementation.go"
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{groundedTarget}
	candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters(groundedTarget)
	if admitted := EvaluateCandidateAdmission(aggregate, candidate); !admitted.Accepted() {
		t.Fatalf("owner-grounded workspace scope candidate rejected before File admissibility: result=%+v failure=%+v", admitted, admitted.Failure)
	}
}

func TestEvaluateCandidateAdmissionRejectsTargetOutsideFiniteOwnerGroundedWorkspaceScope(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.TargetSelectorRef = "file/workspace_deliverable"
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs: []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	runtimeTestSealCurrentFocusBindingTargetSurface(&aggregate.Facts, runtimeTestBindingTargetSurface{
		SelectorRef: selection.SelectorRef, SelectorOwnerRef: selection.OwnerModuleID, SelectorSubjectRef: selection.TargetRole,
	})
	bindCurrentFocusTargetSelection(&aggregate.Facts)
	addCandidateAdmissionTargetPolicyFixture(&aggregate.Facts, runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeBlockKindPatchApply, "file_write", runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, selection.TargetRefs)

	candidate := stateChangeCandidate()
	outsideTarget := "new-deliverable.go"
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{outsideTarget}
	candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters(outsideTarget)
	admitted := EvaluateCandidateAdmission(aggregate, candidate)
	if admitted.Disposition != CandidateAdmissionRejectedContract || admitted.Failure == nil || admitted.Failure.Code != candidateAdmissionCodeInvalid {
		t.Fatalf("outside finite owner scope admission = %+v", admitted)
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
	runtimeTestSealCurrentFocusBindingTargetSurface(&aggregate.Facts, runtimeTestBindingTargetSurface{
		SelectorRef: selection.SelectorRef, SelectorOwnerRef: selection.OwnerModuleID, SelectorSubjectRef: selection.TargetRole,
	})
	bindCurrentFocusTargetSelection(&aggregate.Facts)

	operation := runtimecontract.RuntimeOperation{ID: "initialize-repository", Kind: runtimecontract.StepKindGitWrite, TargetRefs: []string{"implementation.go"}}
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: operation}}}}); result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding {
		t.Fatalf("foreign-owner workspace-scope candidate admission = %+v", result)
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
	addCandidateAdmissionTargetPolicyFixture(&aggregate.Facts, runtimecontract.RuntimeModuleShell, runtimecontract.StepKindShellCommand, "", runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, selection.TargetRefs)

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
	aggregate.Facts.Focus.ProspectiveTargetRefs = []string{"new-deliverable.txt"}
	aggregate.Facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-create", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable",
		SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: "new-deliverable.txt", CandidateCount: 0,
		ObservedRevision: "workspace-tree-empty",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	bindCurrentFocusTargetSelection(&aggregate.Facts)
	addCandidateAdmissionTargetPolicyFixture(&aggregate.Facts, runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeBlockKindPatchApply, "file_write", runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, nil)

	gitInit := "init"
	nonFileCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "initialize-repository", Kind: runtimecontract.StepKindGitWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &gitInit}},
	}}}}}
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, nonFileCandidate); result.Disposition != CandidateAdmissionRejectedContract || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeTargetOutsideGrounding {
		t.Fatalf("foreign-owner creation-scope candidate admission = %+v", result)
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
	runtimeTestSealCurrentFocusBindingTargetSurface(&aggregate.Facts, runtimeTestBindingTargetSurface{
		SelectorRef: selection.SelectorRef, SelectorOwnerRef: selection.OwnerModuleID, SelectorSubjectRef: selection.TargetRole,
	})
	bindCurrentFocusTargetSelection(&aggregate.Facts)
	aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
		Signature: "owner-observed-failure", TargetRefs: []string{"implementation.go"},
	}
	addCandidateAdmissionTargetPolicyFixture(&aggregate.Facts, runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeBlockKindPatchApply, "file_write", runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, selection.TargetRefs)

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
		t.Fatalf("candidate targeting owner failure target rejected: result=%+v failure=%+v", result, result.Failure)
	}
}

func TestAdmitCandidateProposalAtomicallyExhaustsSingleShapeFrontier(t *testing.T) {
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
	currentAttempt := first.Aggregate.CandidateAttempts[len(first.Aggregate.CandidateAttempts)-1]
	if first.Admission.Failure == nil || first.Admission.Failure.Code != candidateAdmissionCodeReplaysRejectedShape || len(first.Aggregate.CandidateAttempts) != 2 || currentAttempt.State != runtimecontract.RuntimeCandidateAttemptStateFailed ||
		currentAttempt.Correction == nil || currentAttempt.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || currentAttempt.Correction.TerminalCode != "candidate_authority_exhausted" ||
		first.Aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || first.Aggregate.Decision.Terminal == nil || first.Aggregate.Decision.Terminal.Code != "candidate_authority_exhausted" {
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
	savesAfterExhaustion := repository.saves
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, "stable-prefix"); err == nil || repository.saves != savesAfterExhaustion {
		t.Fatalf("exhausted single-shape frontier opened attempt 1: err=%v saves=%d", err, repository.saves)
	}
}

func TestAdmitCandidateProposalReturnsCommittedDuplicateAfterAdministrativeTermination(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "terminal-duplicate-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimeTestBindCandidateToCurrentFocus(t, aggregate, stateChangeCandidate())
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{"other.go"}
	first, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || first.Admission.Failure == nil || first.Admission.Correction == nil ||
		first.Admission.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce ||
		first.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("correctable proposal setup=%+v err=%v", first, err)
	}
	terminated, err := controller.requestTermination(
		context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID,
		runtimecontract.RuntimeTerminationCancelled, "blocks/run", "test response-loss terminal fence",
	)
	if err != nil || terminated.Termination == nil {
		t.Fatalf("administrative termination=%+v err=%v", terminated, err)
	}
	saves := repository.saves
	duplicate, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || duplicate.Admission.Failure == nil || duplicate.Admission.Failure.Code != first.Admission.Failure.Code ||
		duplicate.Aggregate.StateRevision != terminated.StateRevision || repository.saves != saves {
		t.Fatalf("terminated exact duplicate=%+v err=%v saves=%d", duplicate, err, repository.saves)
	}
	different := runtimeTestBindCandidateToCurrentFocus(t, aggregate, stateChangeCandidate())
	different.StateChange.Operations[0].Operation.TargetRefs = []string{"different.go"}
	if _, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, different); err == nil || repository.saves != saves {
		t.Fatalf("terminated non-duplicate err=%v saves=%d, want conflict and no CAS", err, repository.saves)
	}
}

func TestAdmitCandidateProposalExhaustsSemanticReplayWithoutOperationIdentity(t *testing.T) {
	newGroundedAggregate := func() RuntimeAggregate {
		return stateChangeAggregateWithPreparation(t, func(initial *RuntimeAggregate) {
			initial.Facts.Focus.RequireTargetGrounding = true
			initial.Facts.Focus.TargetRole = "implementation"
			initial.Facts.Focus.TargetRefs = []string{"implementation.go"}
		})
	}
	candidateForTarget := func(aggregate RuntimeAggregate, operationID, targetRef string) runtimecontract.RuntimeCandidate {
		candidate := stateChangeCandidate()
		candidate.StateChange.Operations[0].Operation.TargetRefs = []string{"implementation.go"}
		candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters("implementation.go")
		candidate = runtimeTestBindCandidateToCurrentFocus(t, aggregate, candidate)
		candidate.StateChange.Operations[0].Operation.ID = operationID
		candidate.StateChange.Operations[0].Operation.TargetRefs = []string{targetRef}
		candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters(targetRef)
		return candidate
	}

	t.Run("semantic replay exhausts exactly once", func(t *testing.T) {
		aggregate := newGroundedAggregate()
		wrongCandidate := candidateForTarget(aggregate, "provider-wrong-id", "main.go")
		equivalent := wrongCandidate
		equivalent.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), wrongCandidate.StateChange.Operations...)}
		equivalent.StateChange.Operations[0].Operation.ID = "provider-rewritten-id"
		parameters := append([]runtimecontract.OperationParameter(nil), equivalent.StateChange.Operations[0].Operation.Parameters...)
		for left, right := 0, len(parameters)-1; left < right; left, right = left+1, right-1 {
			parameters[left], parameters[right] = parameters[right], parameters[left]
		}
		equivalent.StateChange.Operations[0].Operation.Parameters = parameters
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
			first.Aggregate.Decision.State == runtimecontract.ConvergenceStateFailed {
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
			second.Aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || second.Aggregate.Decision.Terminal == nil || second.Aggregate.Decision.Terminal.Code != "candidate_retry_exhausted" ||
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
		wrongCandidate := candidateForTarget(aggregate, "provider-wrong-id", "main.go")
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
		retargeted := wrongCandidate
		retargeted.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), wrongCandidate.StateChange.Operations...)}
		retargeted.StateChange.Operations[0].Operation.ID = "provider-corrected-id"
		retargeted.StateChange.Operations[0].Operation.TargetRefs = []string{"implementation.go"}
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		second, err := controller.AdmitCandidateProposalForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, retargeted)
		if err != nil || !second.Admission.Accepted() || second.Aggregate.Decision.State == runtimecontract.ConvergenceStateFailed || hasPendingCandidateAttempt(second.Aggregate.CandidateAttempts) == false {
			t.Fatalf("legal retarget was incorrectly classified as semantic replay: failure=%+v correction=%+v decision=%+v err=%v", second.Admission.Failure, second.Admission.Correction, second.Aggregate.Decision, err)
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
		if err := RecordFixtureCandidateFeasibilities(
			context.Background(), controller, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate,
		); err != nil {
			t.Fatal(err)
		}
		first, err := controller.AdmitAndSealCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
		if err != nil || !first.Admission.Accepted() || first.Aggregate.ActiveDispatch == nil {
			t.Fatalf("first seal = %+v err=%v", first, err)
		}
		if len(first.Aggregate.CandidateAttempts) != 1 ||
			!runtimecontract.RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(
				first.Aggregate.CandidateAttempts[0].AdmissionTargetAuthorizations,
				first.Aggregate.ActiveDispatch.OperationTargetAuthorizations,
			) ||
			first.Aggregate.CandidateAttempts[0].TargetAuthorizationHash != runtimecontract.StableRuntimeDispatchOperationTargetAuthorizationsHash(
				first.Aggregate.CandidateAttempts[0].AdmissionTargetAuthorizations,
			) {
			t.Fatalf("accepted attempt did not persist its independent admission target projection: %+v", first.Aggregate.CandidateAttempts)
		}
		if len(first.Aggregate.ActiveDispatch.OperationExecutions) != 1 ||
			first.Aggregate.ActiveDispatch.OperationExecutions[0].AdmissibilitySnapshotID == "" ||
			len(first.Aggregate.ActiveDispatch.OperationAdmissibilities) != 1 {
			t.Fatalf("accepted attempt did not seal its exact feasible owner catalog: %+v", first.Aggregate.ActiveDispatch)
		}
		saves := repository.saves
		duplicate, err := controller.AdmitAndSealCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
		if err != nil || !duplicate.Admission.Accepted() || duplicate.Aggregate.ActiveDispatch == nil || duplicate.Aggregate.StateRevision != first.Aggregate.StateRevision || repository.saves != saves {
			t.Fatalf("duplicate seal = %+v err=%v saves=%d", duplicate, err, repository.saves)
		}
	})
}

func aggregateWithActiveRejectedCandidate(t *testing.T) (RuntimeAggregate, runtimecontract.RuntimeCandidate) {
	return aggregateWithActiveRejectedCandidateWithPreparation(t, nil)
}

func aggregateWithActiveRejectedCandidateWithPreparation(t *testing.T, prepare func(*RuntimeAggregate)) (RuntimeAggregate, runtimecontract.RuntimeCandidate) {
	t.Helper()
	aggregate := stateChangeAggregateWithPreparation(t, prepare)
	candidate := stateChangeCandidate()
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	aggregate = sealCandidateForTest(t, repository, candidate)
	candidate = aggregate.ActiveDispatch.Candidate
	operationID := runtimeTestCandidateOperationID(t, candidate, 0)
	evidence := ExecutionEvidence{
		EventID: "rejected-patch", WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DispatchID: aggregate.ActiveDispatch.ID, AuthoritySnapshotID: aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: operationID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionRejected, Code: "rejected-patch",
			Signature: "rejected-patch", Message: "owner rejected patch operation",
		},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatalf("commit rejected candidate-shape receipt: %v", err)
	}
	if len(committed.Aggregate.Facts.Evidence.Rejected) != 1 || committed.Aggregate.Facts.Evidence.Rejected[0].FailureSignature != "rejected-patch" {
		t.Fatalf("canonical rejected candidate shape = %+v", committed.Aggregate.Facts.Evidence.Rejected)
	}
	return committed.Aggregate, candidate
}

func addCandidateAdmissionTargetPolicyFixture(facts *runtimecontract.RuntimeFacts, owner, kind, variantID string, policy runtimecontract.RuntimeOperationTargetPolicy, targetRefs []string) {
	selectorRefs := []string{"file/workspace_deliverable"}
	constraintRefs := []string{"workspace-relative-path"}
	variants := []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind: kind, VariantID: variantID, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{policy},
		SelectorRefs: selectorRefs, ConstraintRefs: constraintRefs,
	}}
	if kind == runtimecontract.RuntimeBlockKindPatchApply && variantID == "file_write" && policy == runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope {
		variants = nil
		for _, patchVariant := range []string{"file_write", "text_replace", "unified_diff"} {
			variants = append(variants, runtimecontract.RuntimeOperationAdmissibilityVariant{
				Kind: kind, VariantID: patchVariant, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{policy},
				SelectorRefs: selectorRefs, ConstraintRefs: constraintRefs,
			})
		}
	}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID:    "operation_admissibility:test:" + kind,
		OwnerModuleID:     owner,
		OperationKinds:    []string{kind},
		TargetRefs:        append([]string(nil), targetRefs...),
		SelectorRefs:      selectorRefs,
		ConstraintRefs:    constraintRefs,
		OperationVariants: variants,
		ObservedRevision:  "owner-policy-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, inventory)
}
