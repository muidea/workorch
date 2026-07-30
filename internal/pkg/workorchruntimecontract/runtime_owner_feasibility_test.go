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
	observation, err := RuntimeOwnerFeasibilityObservationFromFailure(RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable, &feedback, failure, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Disposition != RuntimeOwnerFeasibilityInfeasible || observation.Category != RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable {
		t.Fatalf("observation = %+v", observation)
	}
	if observation.Code != "validation_target_missing" {
		t.Fatalf("owner feasibility code = %q, want owner-local observation code", observation.Code)
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
	observation := RuntimeOwnerFeasibilityObservation{Disposition: RuntimeOwnerFeasibilityFeasible, Code: "must-not-appear"}
	if err := observation.Validate(); err == nil {
		t.Fatal("feasible observation accepted failure detail")
	}
}
