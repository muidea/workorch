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
	conflicting := append(policies, RuntimeValidationPolicy{
		ID: "other", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{EvidenceKindValidation}, MaxValidationOnlyRetriesPerFocus: 3,
	})
	if err := ValidateRuntimeValidationPolicies(conflicting); err == nil {
		t.Fatal("expected conflicting validation retry policy to fail")
	}
}

func TestRuntimeValidationEvidenceRecipeRequiresObservableArguments(t *testing.T) {
	policy := RuntimeEvidencePolicy{
		ID: "go-test-observable", AppliesTo: StepKindShellCommand, CommandName: "go",
		RequiredArgs: []string{"test", "-v"},
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("observable recipe policy rejected: %v", err)
	}
	if err := (RuntimeEvidencePolicy{ID: "invalid", AppliesTo: StepKindShellCommand, RequiredArgs: []string{"-v"}}).Validate(); err == nil {
		t.Fatal("required recipe args without a command name must be rejected")
	}
}

func TestRequiredRuntimeValidationDiscoverySourcesArePureAndScoped(t *testing.T) {
	policies := []RuntimeValidationPolicy{
		{ID: "validation", Scope: "workspace", AppliesToEvidenceKinds: []string{EvidenceKindValidation}, DiscoveryRequired: true,
			DiscoverySources: []string{RuntimeValidationDiscoverySourceProjectFiles, RuntimeValidationDiscoverySourceWorkspaceManifest}},
		{ID: "state", Scope: "workspace", AppliesToEvidenceKinds: []string{EvidenceKindStateChange}, DiscoveryRequired: true,
			DiscoverySources: []string{RuntimeValidationDiscoverySourceAvailableCapabilities}},
	}
	got := RequiredRuntimeValidationDiscoverySources(policies, EvidenceKindValidation)
	want := []string{RuntimeValidationDiscoverySourceProjectFiles, RuntimeValidationDiscoverySourceWorkspaceManifest}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("validation discovery sources = %v, want %v", got, want)
	}
}
