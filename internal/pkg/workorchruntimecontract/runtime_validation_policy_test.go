package workorchruntimecontract

import "testing"

func TestRuntimeValidationPolicyRetryLimitIsTypedAndDeterministic(t *testing.T) {
	policies := []RuntimeValidationPolicy{{
		ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{EvidenceKindValidation},
		MaxValidationOnlyRetriesPerFocus: 2, DiscoveryRequired: true, ValidationScratchRequired: true,
	}}
	if err := ValidateRuntimeValidationPolicies(policies); err != nil {
		t.Fatal(err)
	}
	if got, ok := RuntimeValidationRetryLimit(policies, EvidenceKindValidation); !ok || got != 2 {
		t.Fatalf("validation retry limit = %d/%v, want 2/true", got, ok)
	}
	if RuntimeValidationFailureRequiresStateChange(policies, EvidenceKindValidation) {
		t.Fatal("policy with false validationFailureImpliesStateChange was ignored")
	}
	conflicting := append(policies, RuntimeValidationPolicy{
		ID: "other", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{EvidenceKindValidation}, MaxValidationOnlyRetriesPerFocus: 3,
	})
	if err := ValidateRuntimeValidationPolicies(conflicting); err == nil {
		t.Fatal("expected conflicting validation retry policy to fail")
	}
}
