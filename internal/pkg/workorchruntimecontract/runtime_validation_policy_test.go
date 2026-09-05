package workorchruntimecontract

import (
	"reflect"
	"strings"
	"testing"
)

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

func TestRuntimeValidationRecipeDeclarationAllowsOwnerAdapterWithoutExecutableAuthority(t *testing.T) {
	policy := RuntimeEvidencePolicy{
		ID: "project-native", AppliesTo: StepKindShellCommand,
		RecipeDeclared: true, RecipeAdapterRef: "shell/project-native-validation/v1", RecipeTimeoutMillis: 60_000,
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("owner-adapted validation declaration rejected: %v", err)
	}
	declarations := RuntimeValidationRecipeDeclarationsFromEvidencePolicies([]RuntimeEvidencePolicy{policy})
	if len(declarations) != 1 || declarations[0].ID != policy.ID || declarations[0].AdapterRef != policy.RecipeAdapterRef || declarations[0].Command != "" {
		t.Fatalf("owner-adapted recipe declaration = %+v", declarations)
	}
	if err := declarations[0].ValidateDeclaration(); err != nil {
		t.Fatalf("owner-adapted recipe declaration became invalid: %v", err)
	}
	if err := declarations[0].Validate(); err == nil {
		t.Fatal("unresolved owner adapter unexpectedly became executable authority")
	}
	if recipes := NormalizeRuntimeValidationRecipes(declarations); len(recipes) != 0 {
		t.Fatalf("unresolved owner adapter leaked into executable recipes: %+v", recipes)
	}

	ambiguous := policy
	ambiguous.CommandName = "go"
	if err := ambiguous.Validate(); err == nil {
		t.Fatal("evidence policy accepted both fixed command and owner adapter")
	}
}

func TestRuntimeActivationRejectsAmbiguousRawEvidencePolicyProjection(t *testing.T) {
	base := RuntimeEvidencePolicy{
		ID: "managed", AppliesTo: StepKindShellCommand, CommandName: "go",
		RecipeDeclared: true, RecipeArgs: []string{"test", "-v", "./..."},
		CoverageRequirements: []RuntimeCoverageEvidenceRequirement{{
			ID: "validation", Scope: "deliverable", RequiredEvidenceKinds: []string{EvidenceKindValidation},
		}},
	}.Normalized()

	tests := map[string][]RuntimeEvidencePolicy{
		"duplicate policy identity": {
			base,
			func() RuntimeEvidencePolicy {
				changed := base
				changed.CommandName = "other"
				return changed
			}(),
		},
		"duplicate nested coverage identity": {
			func() RuntimeEvidencePolicy {
				changed := base
				changed.CoverageRequirements = append(changed.CoverageRequirements, changed.CoverageRequirements[0])
				return changed
			}(),
		},
		"whitespace alias nested coverage identity": {
			func() RuntimeEvidencePolicy {
				changed := base
				alias := changed.CoverageRequirements[0]
				alias.ID = " validation "
				changed.CoverageRequirements = append(changed.CoverageRequirements, alias)
				return changed
			}(),
		},
	}
	for name, policies := range tests {
		t.Run(name, func(t *testing.T) {
			activation := RuntimeActivationContext{EvidencePolicies: policies}
			err := activation.Validate()
			if err == nil || !strings.Contains(err.Error(), "evidence polic") {
				t.Fatalf("ambiguous raw evidence policies accepted: %v", err)
			}
		})
	}
}

func TestCloneRuntimeEvidencePoliciesPreservesCanonicalEmptyOperationVariants(t *testing.T) {
	policies := NormalizeRuntimeEvidencePolicies([]RuntimeEvidencePolicy{{
		ID:        "policy-1",
		AppliesTo: StepKindShellCommand,
		CoverageRequirements: []RuntimeCoverageEvidenceRequirement{{
			ID:    "requirement-1",
			Scope: "direct",
		}},
	}})
	if err := ValidateRuntimeEvidencePolicies(policies); err != nil {
		t.Fatalf("canonical policy fixture: %v", err)
	}
	if policies[0].CoverageRequirements[0].AcceptedOperationVariants == nil {
		t.Fatal("canonical empty operation variants unexpectedly use nil representation")
	}

	cloned := CloneRuntimeEvidencePolicies(policies)
	if !reflect.DeepEqual(cloned, policies) {
		t.Fatalf("clone changed canonical raw representation:\nsource=%#v\nclone=%#v", policies, cloned)
	}
	if err := ValidateRuntimeEvidencePolicies(cloned); err != nil {
		t.Fatalf("cloned canonical policy: %v", err)
	}

	malformed := CloneRuntimeEvidencePolicies(policies)
	malformed[0].CoverageRequirements[0].AcceptedOperationVariants = nil
	clonedMalformed := CloneRuntimeEvidencePolicies(malformed)
	if !reflect.DeepEqual(clonedMalformed, malformed) {
		t.Fatalf("clone laundered malformed raw representation:\nsource=%#v\nclone=%#v", malformed, clonedMalformed)
	}
	if err := ValidateRuntimeEvidencePolicies(clonedMalformed); err == nil {
		t.Fatal("cloned malformed policy unexpectedly passed raw validation")
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
