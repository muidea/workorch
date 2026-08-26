package shell

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellProspectiveTargetConstraintPublishesSufficientGoTestAllOf(t *testing.T) {
	const revision = "shell-policy-test"
	for _, test := range []struct {
		name        string
		recipe      runtimecontract.RuntimeValidationRecipe
		currentRefs []string
		want        bool
		match       string
		reject      string
		wantPrefix  string
	}{
		{
			name: "recursive workspace", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test", Command: "go", Args: []string{"test", "./..."}},
			currentRefs: []string{"go.mod", "main.go"}, want: true, match: "runtime_test.go", reject: "vendor/runtime_test.go",
		},
		{
			name: "test without root module", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-no-module", Command: "go", Args: []string{"test", "./..."}},
			currentRefs: []string{"main.go", "runtime_test.go"}, want: false,
		},
		{
			name: "missing package root", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-pkg", Command: "go", Args: []string{"test", "./internal/example/..."}},
			currentRefs: []string{"go.mod", "main.go"}, want: true, match: "internal/example/runtime_test.go", reject: "runtime_test.go", wantPrefix: "internal/example/",
		},
		{
			name: "existing package root", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-pkg-existing", Command: "go", Args: []string{"test", "./internal/example/..."}},
			currentRefs: []string{"go.mod", "internal/example/main.go"}, want: true, match: "runtime_test.go", reject: "runtime.go",
		},
		{
			name: "incomparable package roots", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-many", Command: "go", Args: []string{"test", "./pkg/a", "./pkg/b"}},
			currentRefs: []string{"go.mod", "main.go"}, want: false,
		},
		{
			name: "non go recipe", recipe: runtimecontract.RuntimeValidationRecipe{ID: "shell-check", Command: "sh", Args: []string{"-c", "true"}},
			currentRefs: []string{"main.go"}, want: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			constraint, ok := shellProspectiveTargetSatisfactionConstraint(test.recipe, test.currentRefs, revision)
			if ok != test.want {
				t.Fatalf("constraint published=%t value=%+v, want %t", ok, constraint, test.want)
			}
			if !ok {
				return
			}
			if err := constraint.Validate(); err != nil {
				t.Fatalf("published constraint invalid: %v", err)
			}
			if !runtimecontract.RuntimeProspectiveTargetSatisfiesConstraint(test.match, constraint) {
				t.Fatalf("published constraint rejected sufficient target %q: %+v", test.match, constraint)
			}
			if runtimecontract.RuntimeProspectiveTargetSatisfiesConstraint(test.reject, constraint) {
				t.Fatalf("published constraint accepted insufficient target %q: %+v", test.reject, constraint)
			}
			if !runtimeValidationRecipeApplicableToManifest(test.recipe, append(append([]string(nil), test.currentRefs...), test.match)) {
				t.Fatalf("constraint-matching target %q did not make recipe applicable", test.match)
			}
			for _, candidate := range []string{
				"runtime_test.go",
				"nested/runtime_test.go",
				"internal/example/runtime_test.go",
				"internal/example/sub/runtime_test.go",
				"pkg/a/runtime_test.go",
				"runtime.go",
				"vendor/runtime_test.go",
				"node_modules/runtime_test.go",
				".workorch-data/runtime_test.go",
				".git/runtime_test.go",
			} {
				if runtimecontract.RuntimeProspectiveTargetSatisfiesConstraint(candidate, constraint) &&
					!runtimeValidationRecipeApplicableToManifest(test.recipe, append(append([]string(nil), test.currentRefs...), candidate)) {
					t.Fatalf("published constraint is not sufficient: matching target %q left recipe inapplicable", candidate)
				}
			}
			if test.wantPrefix != "" && !shellConstraintHasPredicate(constraint, runtimecontract.RuntimeProspectiveTargetPredicatePathPrefix, test.wantPrefix) {
				t.Fatalf("published constraint omitted package prefix %q: %+v", test.wantPrefix, constraint)
			}
		})
	}
}

func TestShellProspectiveTargetConstraintKeepsMultipleGoWorkspaceGapsNonActionable(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "go-test", Command: "go", Args: []string{"test", "-v", "./..."}}
	currentRefs := []string{"main.go"}
	if constraint, ok := shellProspectiveTargetSatisfactionConstraint(recipe, currentRefs, "shell-policy-test"); ok {
		t.Fatalf("single-target contract published for missing go.mod and test input: %+v", constraint)
	}
	for _, target := range []string{"go.mod", "runtime_test.go"} {
		if runtimeValidationRecipeApplicableToManifest(recipe, append(append([]string(nil), currentRefs...), target)) {
			t.Fatalf("single target %q unexpectedly satisfied both module and test prerequisites", target)
		}
	}
}

func shellConstraintHasPredicate(constraint runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint, kind runtimecontract.RuntimeProspectiveTargetPredicateKind, value string) bool {
	for _, predicate := range constraint.Normalized().AllOf {
		if predicate.Kind == kind && predicate.Value == value {
			return true
		}
	}
	return false
}
