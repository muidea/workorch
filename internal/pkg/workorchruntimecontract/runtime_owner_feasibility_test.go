package workorchruntimecontract

import "testing"

func TestRuntimeOwnerFeasibilityObservationSeparatesOwnerFactsFromCandidateCorrection(t *testing.T) {
	failure := &FailureFact{
		Class: FailureClassCandidateContractFailure, Code: "validation_target_missing",
		Signature: "shell/validation-target-missing", Message: "no validation target exists", TargetRefs: []string{"main_test.go"},
	}
	feedback, err := RuntimeFeedbackFromFailureFact(RuntimeFeedbackScope{WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: "authority-v2"}, RuntimeFeedbackDomainFeasibility, RuntimeFeedbackCategory(RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable), RuntimeModuleShell, *failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = "validate-1"
	observation, err := RuntimeOwnerFeasibilityObservationFromOutcome(RuntimeOwnerFeasibilityInfeasible, &feedback, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Outcome != RuntimeOwnerFeasibilityInfeasible || observation.FailureCategory() != RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable {
		t.Fatalf("observation = %+v", observation)
	}
	if observation.Feedback == nil || observation.Feedback.OriginalDiagnostic == nil || observation.Feedback.OriginalDiagnostic.OriginalCode != "validation_target_missing" {
		t.Fatalf("owner feasibility lost original diagnostic: %+v", observation.Feedback)
	}
}

func TestRuntimeOwnerFeasibilityCategoryVocabulary(t *testing.T) {
	for _, category := range []RuntimeOwnerFeasibilityCategory{
		RuntimeOwnerFeasibilityCategoryResourceAbsent,
		RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable,
		RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
		RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
		RuntimeOwnerFeasibilityCategoryTargetNotRegular,
		RuntimeOwnerFeasibilityCategoryObservationUnknown,
		RuntimeOwnerFeasibilityCategoryOwnerContractDefect,
	} {
		if !ValidRuntimeOwnerFeasibilityCategory(category) {
			t.Fatalf("declared owner-neutral category is invalid: %q", category)
		}
	}
	if ValidRuntimeOwnerFeasibilityCategory("new_unmapped_category") {
		t.Fatal("unknown category accepted")
	}
}

func TestRuntimeOwnerFeasibilityObservationValidatesExclusiveOutcomes(t *testing.T) {
	observation := RuntimeOwnerFeasibilityObservation{Outcome: RuntimeOwnerFeasibilityFeasible, Feedback: &RuntimeFeedback{}}
	if err := observation.Validate(); err == nil {
		t.Fatal("feasible observation accepted failure detail")
	}
}

func TestRuntimeOwnerFeasibilityConstraintImpactRequiresExplicitRevisionBoundOwnerFact(t *testing.T) {
	failure := FailureFact{
		Class: FailureClassCandidateContractFailure, Code: "operation_target_exists", Signature: "file/target-exists",
		Message: "file/write is create-only", TargetRefs: []string{"target.txt"},
	}
	feedback, err := RuntimeFeedbackFromFailureFact(
		RuntimeFeedbackScope{WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1"},
		RuntimeFeedbackDomainFeasibility, RuntimeFeedbackCategory(RuntimeOwnerFeasibilityCategoryPolicyDisallowed), RuntimeModuleFile, failure,
	)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = "write-1"
	feedback.OriginalDiagnostic.ObservedRevision = "tree-v2"
	observation := RuntimeOwnerFeasibilityObservation{
		Outcome: RuntimeOwnerFeasibilityInfeasible, Feedback: &feedback,
		OwnerPolicy:      RuntimeOwnerPolicyIdentity{OwnerModuleID: RuntimeModuleFile, PolicyRevision: "file-policy-v1", CatalogSnapshotID: "catalog-v2"},
		ConstraintImpact: RuntimeOwnerFeasibilityConstraintImpactSealedTargetAuthorizationContradicted,
	}
	if err := observation.Validate(); err != nil || !observation.SealedTargetAuthorizationContradicted() {
		t.Fatalf("explicit owner contradiction rejected: observation=%+v err=%v", observation, err)
	}

	candidateLocal := observation
	candidateLocal.ConstraintImpact = ""
	if err := candidateLocal.Validate(); err != nil || candidateLocal.SealedTargetAuthorizationContradicted() {
		t.Fatalf("candidate-local observation was promoted to contradiction: observation=%+v err=%v", candidateLocal, err)
	}

	missingRevision := observation.Clone()
	missingRevision.Feedback.OriginalDiagnostic.ObservedRevision = ""
	if err := missingRevision.Validate(); err == nil {
		t.Fatal("constraint impact without an observed resource revision was accepted")
	}
	wrongCategory := observation.Clone()
	wrongCategory.Feedback.StableCategory = RuntimeFeedbackCategory(RuntimeOwnerFeasibilityCategoryObservationUnknown)
	if err := wrongCategory.Validate(); err == nil {
		t.Fatal("observation-unknown failure was accepted as a sealed target contradiction")
	}
	unknownImpact := observation
	unknownImpact.ConstraintImpact = "owner_private_transition"
	if err := unknownImpact.Validate(); err == nil {
		t.Fatal("unknown owner constraint impact was accepted")
	}
}
