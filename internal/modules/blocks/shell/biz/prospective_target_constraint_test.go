package shell

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellProspectiveTargetConstraintPublishesSufficientGoTestAllOfTargetSet(t *testing.T) {
	const revision = "shell-policy-test"
	for _, test := range []struct {
		name        string
		recipe      runtimecontract.RuntimeValidationRecipe
		currentRefs []string
		want        bool
		match       []string
		reject      []string
		wantPrefix  string
	}{
		{
			name: "recursive workspace", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test", Command: "go", Args: []string{"test", "./..."}},
			currentRefs: []string{"go.mod", "main.go"}, want: true, match: []string{"runtime_test.go"}, reject: []string{"vendor/runtime_test.go"},
		},
		{
			name: "test without root module", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-no-module", Command: "go", Args: []string{"test", "./..."}},
			currentRefs: []string{"main.go", "runtime_test.go"}, want: true, match: []string{"go.mod"}, reject: []string{"nested/go.mod"},
		},
		{
			name: "empty workspace", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-empty", Command: "go", Args: []string{"test", "./..."}},
			want: true, match: []string{"go.mod", "runtime_test.go"}, reject: []string{"runtime_test.go"},
		},
		{
			name: "source only", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-source", Command: "go", Args: []string{"test", "./..."}},
			currentRefs: []string{"main.go"}, want: true, match: []string{"go.mod", "runtime_test.go"}, reject: []string{"go.mod"},
		},
		{
			name: "module only", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-module", Command: "go", Args: []string{"test", "./..."}},
			currentRefs: []string{"go.mod"}, want: true, match: []string{"runtime_test.go"}, reject: []string{"runtime.go"},
		},
		{
			name: "ready workspace", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-ready", Command: "go", Args: []string{"test", "./..."}},
			currentRefs: []string{"go.mod", "runtime_test.go"}, want: false,
		},
		{
			name: "missing package root", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-pkg", Command: "go", Args: []string{"test", "./internal/example/..."}},
			currentRefs: []string{"go.mod", "main.go"}, want: true, match: []string{"internal/example/runtime_test.go"}, reject: []string{"runtime_test.go"}, wantPrefix: "internal/example/",
		},
		{
			name: "existing package root", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-pkg-existing", Command: "go", Args: []string{"test", "./internal/example/..."}},
			currentRefs: []string{"go.mod", "internal/example/main.go"}, want: true, match: []string{"runtime_test.go"}, reject: []string{"runtime.go"},
		},
		{
			name: "incomparable package roots", recipe: runtimecontract.RuntimeValidationRecipe{ID: "go-test-many", Command: "go", Args: []string{"test", "./pkg/a", "./pkg/b"}},
			currentRefs: []string{"go.mod", "main.go"}, want: true,
			match: []string{"pkg/a/runtime_test.go", "pkg/b/runtime.go"}, reject: []string{"pkg/a/runtime_test.go"},
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
			if !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(test.match, constraint) {
				t.Fatalf("published constraint rejected sufficient target set %q: %+v", test.match, constraint)
			}
			if runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(test.reject, constraint) {
				t.Fatalf("published constraint accepted insufficient target set %q: %+v", test.reject, constraint)
			}
			if !runtimeValidationRecipeApplicableToManifest(test.recipe, append(append([]string(nil), test.currentRefs...), test.match...)) {
				t.Fatalf("constraint-matching target set %q did not make recipe applicable", test.match)
			}
			if test.wantPrefix != "" && !shellConstraintHasPredicate(constraint, runtimecontract.RuntimeProspectiveTargetPredicatePathPrefix, test.wantPrefix) {
				t.Fatalf("published constraint omitted package prefix %q: %+v", test.wantPrefix, constraint)
			}
		})
	}
}

func TestShellProspectiveTargetConstraintCoversMultipleGoWorkspaceGapsInOneBoundedSet(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "go-test", Command: "go", Args: []string{"test", "-v", "./..."}}
	currentRefs := []string{"main.go"}
	constraint, ok := shellProspectiveTargetSatisfactionConstraint(recipe, currentRefs, "shell-policy-test")
	if !ok {
		t.Fatal("bounded target-set contract was not published for missing go.mod and test input")
	}
	targets := []string{"go.mod", "runtime_test.go"}
	if !runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(targets, constraint) ||
		!runtimeValidationRecipeApplicableToManifest(recipe, append(append([]string(nil), currentRefs...), targets...)) {
		t.Fatalf("bounded target set did not close all Go workspace gaps: %+v", constraint)
	}
}

func shellConstraintHasPredicate(constraint runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint, kind runtimecontract.RuntimeProspectiveTargetPredicateKind, value string) bool {
	for _, clause := range constraint.Normalized().AllOf {
		for _, predicate := range clause.AllOf {
			if predicate.Kind == kind && predicate.Value == value {
				return true
			}
		}
	}
	return false
}
