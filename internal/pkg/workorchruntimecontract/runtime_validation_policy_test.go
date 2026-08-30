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

func TestRuntimeEvidencePolicySelectorCandidatesRequireGroundingAndCompleteSelector(t *testing.T) {
	policy := RuntimeEvidencePolicy{
		ID: "candidate-selector", AppliesTo: StepKindFileWrite,
		CoverageRequirements: []RuntimeCoverageEvidenceRequirement{{
			ID: "deliverable", Scope: "deliverable", TargetRole: "deliverable", RequireTargetGrounding: true,
			TargetDiscoverySources: []string{RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef:      "file/profile-declared-candidates", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "deliverable",
			TargetSelectorCandidateRefs:          []string{"README.md", "internal/main.go"},
			ProspectiveTargetAllOfConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"},
		}},
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("valid bounded candidate selector rejected: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeCoverageEvidenceRequirement){
		"missing grounding":        func(value *RuntimeCoverageEvidenceRequirement) { value.RequireTargetGrounding = false },
		"missing selector ref":     func(value *RuntimeCoverageEvidenceRequirement) { value.TargetSelectorRef = "" },
		"missing selector owner":   func(value *RuntimeCoverageEvidenceRequirement) { value.TargetSelectorOwnerRef = "" },
		"missing selector subject": func(value *RuntimeCoverageEvidenceRequirement) { value.TargetSelectorSubjectRef = "" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := CloneRuntimeEvidencePolicies([]RuntimeEvidencePolicy{policy})[0]
			mutate(&invalid.CoverageRequirements[0])
			if err := invalid.Validate(); err == nil {
				t.Fatalf("coverage requirement accepted selector candidates without grounding and a complete selector: %+v", invalid.CoverageRequirements[0])
			}
		})
	}
}

func TestRuntimeValidationRecipeDeclarationIsExplicitAndAllowsNoArguments(t *testing.T) {
	policy := RuntimeEvidencePolicy{
		ID: "no-arg-check", AppliesTo: StepKindShellCommand, CommandName: "true",
		RecipeDeclared: true,
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("no-argument validation recipe declaration rejected: %v", err)
	}
	recipes := RuntimeValidationRecipeDeclarationsFromEvidencePolicies([]RuntimeEvidencePolicy{policy})
	if len(recipes) != 1 || recipes[0].ID != "no-arg-check" || len(recipes[0].Args) != 0 {
		t.Fatalf("no-argument recipe declaration = %+v", recipes)
	}
	if err := recipes[0].Validate(); err != nil {
		t.Fatalf("declared no-argument recipe became invalid: %v", err)
	}

	undeclared := RuntimeEvidencePolicy{
		ID: "implicit", AppliesTo: StepKindShellCommand, CommandName: "go",
		RecipeArgs: []string{"test", "./..."},
	}
	if err := undeclared.Validate(); err == nil {
		t.Fatal("recipe fields without an explicit recipe declaration were accepted")
	}
	if recipes := RuntimeValidationRecipeDeclarationsFromEvidencePolicies([]RuntimeEvidencePolicy{undeclared}); len(recipes) != 0 {
		t.Fatalf("undeclared recipe leaked into activation declarations: %+v", recipes)
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
