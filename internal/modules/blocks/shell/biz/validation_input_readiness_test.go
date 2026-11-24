package shell

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Test the complete producer output, including the exact deadline appended by
// failure diagnostics. A bounded generation declaration alone does not prove
// the conjunctive supplement can cross the same consumer boundary.
func TestProjectSupplementCompleteDeclarationFitsOriginalEnvelope(t *testing.T) {
	for _, adapter := range []string{modulecommon.ValidationRecipeAdapterProjectConjunction, modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic, modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic} {
		for _, timeout := range []int64{0, 1000, 300000, 600000} {
			t.Run(fmt.Sprintf("%s/%d", adapter, timeout), func(t *testing.T) {
				recipe := runtimecontract.RuntimeValidationRecipe{ID: "project", Command: "go", Args: []string{"test", "-v", "./..."}, AdapterRef: adapter, TimeoutMillis: timeout, InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol}
				requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-supplement")
				checks, ok := shellValidationCheckAttestations(recipe, requirement)
				if !ok || len(checks) == 0 {
					t.Fatal("missing exact owner check")
				}
				addition := shellValidationInputAddition(checks[0])
				addition.ContentRequirements += shellValidationDeclaredDeadlineRequirement(timeout)
				if err := addition.Validate(); err != nil {
					t.Fatalf("complete supplement (%d bytes) exceeds original boundary: %v", len(addition.ContentRequirements), err)
				}
				if addition.ContentContractRef != shellProjectSupplementContentContractRef {
					t.Fatalf("supplement contentContractRef = %q, want %q", addition.ContentContractRef, shellProjectSupplementContentContractRef)
				}
				expected := []string{shellProjectSupplementContentRequirements, shellAssertionFeedbackRequirements, shellValidationDeclaredDeadlineRequirement(timeout), "NO receiver methods", "NO other Test functions", "all original checks and inputs all-of"}
				if adapter == modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic {
					expected = append(expected, shellNativeSourceExecutionRequirements)
				}
				for _, text := range expected {
					if !strings.Contains(addition.ContentRequirements, text) {
						t.Fatalf("supplement omitted required contract text %q", text)
					}
				}
			})
		}
	}
	for _, timeout := range []int64{0, 1000, 300000, 600000} {
		contract := runtimecontract.RuntimeValidationContentContract{
			Ref:          shellProjectMultiFailureArtifactContentContractRef,
			Requirements: strings.TrimSpace(shellProjectContentRequirements(modulecommon.ValidationRecipeAdapterProjectMultiFailureArtifact) + shellValidationDeclaredDeadlineRequirement(timeout) + shellAssertionFeedbackRequirements),
		}.Normalized()
		if err := contract.Validate(); err != nil {
			t.Fatalf("artifact content contract (%d bytes) exceeds envelope: %v", len(contract.Requirements), err)
		}
	}
	if len(shellProjectMultiFailureArtifactReviewRequirements) > runtimecontract.RuntimeValidationInputReviewMaxScopeBytes {
		t.Fatalf("artifact review scope (%d bytes) exceeds max %d", len(shellProjectMultiFailureArtifactReviewRequirements), runtimecontract.RuntimeValidationInputReviewMaxScopeBytes)
	}
	for _, scope := range []struct{ name, text string }{
		{"isolated", shellIsolatedReviewScopeRequirements},
		{"conjunctive", shellConjunctiveReviewScopeRequirements},
		{"project", shellProjectReviewScopeRequirements},
		{"multi-failure", shellProjectMultiFailureRequirements},
		{"artifact", shellProjectMultiFailureArtifactReviewRequirements},
	} {
		if len(scope.text) > runtimecontract.RuntimeValidationInputReviewMaxScopeBytes {
			t.Fatalf("%s review scope (%d bytes) exceeds %d", scope.name, len(scope.text), runtimecontract.RuntimeValidationInputReviewMaxScopeBytes)
		}
		for _, token := range []string{"-trimpath", "os.Getwd()", "t.TempDir()"} {
			if !strings.Contains(scope.text, token) {
				t.Fatalf("%s review scope omits %q", scope.name, token)
			}
		}
	}
	for _, content := range []struct {
		name, ref, text string
		behavioral      bool
	}{
		{"isolated", shellIsolatedInputContentContractRef, shellIsolatedInputContentRequirements + shellAssertionFeedbackRequirements, true},
		{"conjunctive", shellConjunctiveInputContentContractRef, shellConjunctiveInputContentRequirements + shellAssertionFeedbackRequirements, true},
		{"project", shellProjectInputContentContractRef, shellProjectInputContentRequirements + shellValidationDeclaredDeadlineRequirement(300000) + shellAssertionFeedbackRequirements, true},
		{"multi-failure", shellProjectInputContentContractRef, shellProjectMultiFailureInputContentRequirements + shellValidationDeclaredDeadlineRequirement(300000) + shellAssertionFeedbackRequirements, true},
		{"artifact", shellProjectMultiFailureArtifactContentContractRef, shellProjectMultiFailureArtifactInputContentRequirements + shellValidationDeclaredDeadlineRequirement(300000) + shellAssertionFeedbackRequirements, false},
		{"supplement", shellProjectSupplementContentContractRef, shellProjectSupplementContentRequirements + shellNativeSourceExecutionRequirements + shellValidationDeclaredDeadlineRequirement(300000) + shellAssertionFeedbackRequirements, true},
	} {
		contract := runtimecontract.RuntimeValidationContentContract{Ref: content.ref, Requirements: strings.TrimSpace(content.text)}.Normalized()
		if err := contract.Validate(); err != nil {
			t.Fatalf("%s content contract (%d bytes) invalid: %v", content.name, len(contract.Requirements), err)
		}
		for _, token := range []string{"-trimpath", "os.Getwd()", "t.TempDir()", "Caller/CallersFrames"} {
			if !strings.Contains(contract.Requirements, token) {
				t.Fatalf("%s content contract omits %q", content.name, token)
			}
		}
		if content.behavioral && !strings.Contains(contract.Requirements, "GOCOVERDIR") {
			t.Fatalf("%s behavioral contract omits GOCOVERDIR clarification", content.name)
		}
	}
}

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
	for _, phrase := range []string{"top-level", "helper", "const", "var", "type", "init", "TestMain", "originalNativeCheckRef", "subprocess", "not solely in helper closures", "WORKORCH_ASSERTION_V1", "diagnostic-incomplete", "never proves execution or pass"} {
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
