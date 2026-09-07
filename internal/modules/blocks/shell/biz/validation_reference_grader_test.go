package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A small reference grader is intentionally independent of the implementation
// variants. It proves behavior, not source layout, names or an exit-code marker.
// These are local owner-contract tests, never deployed DCE evidence.
func TestReferenceValidationGraderRejectsWeakAndUnrelatedEvidence(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "reference-behavior", Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: 60_000}
	requirement := shellValidationProofRequirementForTest(t, "", "cwi-reference-behavior")
	attestations, supported := shellValidationCheckAttestations(recipe, requirement)
	if !supported || len(attestations) != 1 {
		t.Fatal("reference fixture has no native adapter")
	}
	native := attestations[0].NativeCheckRef
	assertions := `for _, input := range []int{-3, 0, 4} { if got := compute(input); got != input+7 { t.Fatalf("compute(%d): got %d, want %d", input, got, input+7) } }`
	proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: runtimecontract.EvidenceKindValidation,
		RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession}, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationRecipeID: recipe.ID, ValidationProofRequirementHash: requirement.Hash, RequiredCheckRefs: requirement.RequiredCheckRefs, ValidationCheckAttestations: attestations}
	for _, test := range []struct {
		name, implementation, body string
		wantPass                   bool
	}{
		{"known good", "func compute(v int) int { return v+7 }", assertions, true},
		{"known good in another legal file", "func compute(v int) int { return v+7 }", assertions, true},
		{"known bad with static markers", "// compute returns input+7; all required checks passed\nfunc compute(v int) int { return 0 }", assertions, false},
		{"implementation not callable", "var compute = 7", assertions, false},
		{"assertions removed", "func compute(v int) int { return 0 }", `_ = compute(0)`, false},
		{"whole check skipped", "func compute(v int) int { return 0 }", `t.Skip("missing prerequisite"); ` + assertions, false},
		{"required subcheck skipped", "func compute(v int) int { return 0 }", `t.Run("required", func(t *testing.T) { t.Skip("missing prerequisite"); ` + assertions + ` })`, false},
		{"missing dependency", "var _ = unavailableDependency\nfunc compute(v int) int { return v+7 }", assertions, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := newGoValidationWorkspace(t, fmt.Sprintf("package fixture\nimport \"testing\"\nfunc %s(t *testing.T) { %s }\nfunc TestUnrelatedB(t *testing.T) { t.Fatal(\"unrelated failure\") }\n", native, test.body))
			implementationRef := "fixture.go"
			if test.name == "known good in another legal file" {
				implementationRef = "another_implementation.go"
			}
			if err := os.WriteFile(filepath.Join(workspace, implementationRef), []byte("package fixture\n"+test.implementation+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspace, ValidationRecipeID: recipe.ID,
				ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
				Command:           recipe.Command, Args: recipe.Args, TimeoutMillis: recipe.TimeoutMillis,
				TargetRefs: []string{"fixture_test.go"}, ValidationScopeTargetRefs: []string{implementationRef, "fixture_test.go", "go.mod"}, ValidationCheckAttestations: attestations}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			// The owner advertises direct proof only for declared native
			// checks. This is its existing structural prerequisite, not a
			// semantic grader. Executing a raw command alone skips that gate.
			declared := shellNativeValidationChecksDeclared(workspace, request.ValidationScopeTargetRefs, attestations)
			result, executeErr := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
			observation := result.Evidence.Validation
			passed := declared && executeErr == nil && observation != nil && runtimecontract.RuntimeValidationObservationSatisfiesCoverageProof(proof, *observation)
			if passed != test.wantPass {
				t.Fatalf("reference proof=%t want=%t err=%v observation=%+v", passed, test.wantPass, executeErr, observation)
			}
			if test.name == "required subcheck skipped" && (!declared || observation == nil || len(observation.IncompleteChecks) == 0) {
				t.Fatal("subcheck skip must be rejected by native observation, not merely declaration shape")
			}
			if test.name == "known bad with static markers" {
				if observation == nil || len(observation.CheckFailures) == 0 || observation.CheckFailures[0].CheckRef != native || observation.CheckFailures[0].TargetRef != "fixture_test.go" {
					t.Fatalf("real process lost the failed assertion location: %+v", observation)
				}
				// A weak repair of B in the same file cannot erase A's failed
				// observation; the receipt/evidence remains unaccepted.
				first := runtimecontract.EvidenceFact{ID: "before", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{requirement.CoverageWorkItemID}, Validation: observation}
				body, err := os.ReadFile(filepath.Join(workspace, "fixture_test.go"))
				if err != nil {
					t.Fatal(err)
				}
				body = []byte(strings.Replace(string(body), `t.Fatal("unrelated failure")`, `t.Skip("unrelated failure")`, 1))
				if err := os.WriteFile(filepath.Join(workspace, "fixture_test.go"), body, 0o644); err != nil {
					t.Fatal(err)
				}
				second, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
				if err == nil || second.Evidence.Validation == nil {
					t.Fatal("unrelated edit hid failed check")
				}
				after := first
				after.ID, after.Validation = "after", second.Evidence.Validation
				if runtimecontract.RuntimeValidationFailureOccurrences(runtimecontract.EvidenceLedger{Rejected: []runtimecontract.EvidenceFact{first, after}}, after) != 2 {
					t.Fatal("unrelated edit erased repeated failure")
				}
			}
		})
	}
}
