package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidationSurfacesReportInputTargetsIndependentlyOfReadiness(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "native", Command: "go", Args: []string{"test", "./..."}, AdapterRef: modulecommon.ValidationRecipeAdapterProjectEntrypoint}
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-input-protection")
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("missing native mapping")
	}
	for _, scenario := range []string{"ready", "ambiguous", "assertion_unresolved", "later_proof_batch"} {
		t.Run(scenario, func(t *testing.T) {
			body := "if false { t.Fatal(\"original criterion\") }"
			if scenario == "assertion_unresolved" {
				body = ""
			}
			input := fmt.Sprintf("package fixture\nimport \"testing\"\nfunc %s(t *testing.T){ %s }", checks[0].NativeCheckRef, body)
			root := newGoValidationWorkspace(t, input)
			files := map[string]string{"fixture_test.go": input, "workorch_validation/later/input_test.go": "invalid historical input"}
			if scenario == "ambiguous" {
				files["duplicate_test.go"] = input
			}
			refs, want := []string{"go.mod"}, []string{"fixture_test.go", "workorch_validation/later/input_test.go"}
			for ref, content := range files {
				writeNativeStructureSource(t, root, ref, content)
				refs = append(refs, ref)
			}
			if scenario == "ambiguous" {
				want = append(want, "duplicate_test.go")
			}
			requirements := []runtimecontract.RuntimeValidationProofRequirement{requirement}
			if scenario == "later_proof_batch" {
				requirements = nil
				want = []string{"workorch_validation/later/input_test.go"}
			}
			sort.Strings(want)
			for range 2 {
				surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, requirements, shellTestWorkspaceManifestBinding("same-tree", refs), "catalog", root)
				if len(surfaces) != 1 || surfaces[0].Validate() != nil || surfaces[0].InputTargetOwnerModuleID != runtimecontract.RuntimeModuleFile || !reflect.DeepEqual(surfaces[0].InputTargetRefs, want) {
					t.Fatalf("readiness lost owner input facts: %+v", surfaces)
				}
				if scenario != "ready" && len(surfaces[0].ProofRequirementBindings) != 0 {
					t.Fatal("input inventory granted validation authority")
				}
			}
			for ref, before := range files {
				after, err := os.ReadFile(filepath.Join(root, ref))
				if err != nil || string(after) != before {
					t.Fatalf("fact collection changed input %s: %v", ref, err)
				}
			}
		})
	}
}
