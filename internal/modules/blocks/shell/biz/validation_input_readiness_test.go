package shell

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSupplementContentContractAndFreshReadinessAgreeWithoutReplacementPreparation(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "native", Command: "go", Args: []string{"test", "./..."}, AdapterRef: modulecommon.ValidationRecipeAdapterProjectNative}
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-supplement")
	other := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-other")
	checks, _ := shellValidationCheckAttestations(recipe, requirement)
	otherChecks, _ := shellValidationCheckAttestations(recipe, other)
	addition := shellValidationInputAddition(checks[0])
	if addition.Validate() != nil || addition.ContentContractRef != shellSupplementContentContractRef {
		t.Fatal("missing bounded owner content contract")
	}
	for _, phrase := range []string{"top-level", "helper", "const", "var", "type", "init", "TestMain", "originalNativeCheckRef", "subprocess", "not solely in helper closures"} {
		if !strings.Contains(addition.ContentRequirements, phrase) {
			t.Fatalf("content contract omits %q", phrase)
		}
	}
	for _, tc := range []struct{ name, extra, body, code string }{
		{"inline", "", "const expected = 1; type response struct { value int }; value := response{expected}; if value.value != 1 { t.Fatal(\"bad\") }", ""},
		{"inline helper", "", "read := func() int { return 1 }; if read() != 1 { t.Fatal(\"bad\") }", ""},
		{"assertion only in helper closure", "", "verify := func() { t.Fatal(\"bad\") }; verify()", "native_check_assertion_unresolved"},
		{"helper", "func helper(t *testing.T) { t.Fatal(\"bad\") }", "", "native_check_supplement_contract_invalid"},
		{"constant", "const expected = 1", "", "native_check_supplement_contract_invalid"},
		{"variable", "var expected = 1", "", "native_check_supplement_contract_invalid"},
		{"type", "type response struct { Value int }", "", "native_check_supplement_contract_invalid"},
		{"runner", "func TestMain(m *testing.M) {}", "", "native_check_supplement_contract_invalid"},
		{"init", "func init() {}", "", "native_check_supplement_contract_invalid"},
		{"syntax", "func invalid(", "", "native_check_source_invalid"},
		{"no assertion", "", "_ = 1", "native_check_assertion_unresolved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeNativeStructureSource(t, root, "original_test.go", fmt.Sprintf("package fixture\nimport \"testing\"\nfunc %s(t *testing.T) { if false { t.Fatal(\"original\") } }\nfunc %s(t *testing.T) { if false { t.Fatal(\"other\") } }", checks[0].NativeCheckRef, otherChecks[0].NativeCheckRef))
			body := tc.body
			if body == "" {
				body = "if false { t.Fatal(\"supplement\") }"
			}
			writeNativeStructureSource(t, root, addition.TargetRef, fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T) { %s }\n%s", addition.NativeCheckRef, body, tc.extra))
			binding := shellTestWorkspaceManifestBinding("tree-supplement", []string{"go.mod", "original_test.go", addition.TargetRef})
			catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "catalog", OwnerModuleID: ModuleID, OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ObservedRevision: "catalog-v3"}
			catalog.ValidationSurfaces = shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationProofRequirement{requirement, other}, binding, catalog.ObservedRevision, root)
			surface := catalog.ValidationSurfaces[0]
			if err := surface.Validate(); err != nil {
				t.Fatal(err)
			}
			if surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady || len(surface.ProofRequirementPreparations) != 0 {
				t.Fatalf("per-CWI inspection changed recipe readiness or offered replacement: %+v", surface)
			}
			if tc.code == "" {
				if len(surface.ProofRequirementBindings) != 2 || len(surface.ProofRequirementUnavailabilities) != 0 {
					t.Fatal("valid inline content did not retain two independent bindings")
				}
				return
			}
			if len(surface.ProofRequirementBindings) != 1 || surface.ProofRequirementBindings[0].RequirementHash != other.Hash || len(surface.ProofRequirementUnavailabilities) != 1 {
				t.Fatal("bad supplemental input affected another CWI or lost exact cause")
			}
			unavailable := surface.ProofRequirementUnavailabilities[0]
			if unavailable.Requirement.Hash != requirement.Hash || unavailable.OriginalDiagnostic.OriginalCode != tc.code || !reflect.DeepEqual(unavailable.OriginalDiagnostic.TargetRefs, []string{addition.TargetRef}) {
				t.Fatalf("fresh owner cause drifted: %+v", unavailable)
			}
			ownerRequirement := runtimecontract.RuntimeOwnerFactRequirement{OwnerModuleID: ModuleID, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface, ValidationProofRequirementHash: requirement.Hash}
			if _, err := runtimecontract.ResolveRuntimeProspectiveTargetSatisfactionConstraint(catalog, ownerRequirement, modulecommon.ProspectiveSatisfactionContractRef); err == nil {
				t.Fatal("unavailability became prospective satisfaction authority")
			}
			request := runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest{OwnerCatalog: catalog, Requirement: ownerRequirement}
			if len(shellProspectiveSelectableValidationRecipeRefs(request)) != 0 {
				t.Fatal("unavailable native input claimed would_satisfy")
			}
		})
	}
}
