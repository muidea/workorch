package shell

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestNativeCheckPreparationReportsExactCauseAndAcceptsReachableHelper(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "native", Command: "go", Args: []string{"test", "./..."}, AdapterRef: modulecommon.ValidationRecipeAdapterProjectNative}
	requirement := shellValidationProofRequirementForTest(t, "", "cwi-helper")
	attestations, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok || len(attestations) != 1 {
		t.Fatal("missing exact native-check fixture")
	}
	check := attestations[0].NativeCheckRef
	workspace := newGoValidationWorkspace(t, fmt.Sprintf("package fixture\nimport \"testing\"\nfunc %s(t *testing.T) { t.Run(\"required\", func(child *testing.T) { verify(child) }) }\n", check))
	manifest := shellTestWorkspaceManifestBinding("tree-helper-missing", []string{"fixture.go", "fixture_test.go", "go.mod"}).Manifest
	preparation, ready, supported := shellValidationProofPreparation(workspace, manifest, "catalog-before", true, recipe, requirement, attestations)
	if ready || !supported || preparation.OriginalDiagnostic == nil {
		t.Fatalf("unresolved helper did not publish a preparation cause: %+v", preparation)
	}
	diagnostic := preparation.OriginalDiagnostic
	if err := preparation.Validate(); err != nil {
		t.Fatal(err)
	}
	if diagnostic.OwnerModuleID != ModuleID || diagnostic.ObservedRevision != "catalog-before" || diagnostic.OriginalCode != "native_check_assertion_unresolved" ||
		!strings.Contains(diagnostic.OriginalMessage, check) || !strings.Contains(diagnostic.OriginalMessage, requirement.Hash) ||
		!reflect.DeepEqual(diagnostic.TargetRefs, []string{"fixture_test.go"}) {
		t.Fatalf("preparation lost exact owner/check/source identity: %+v", diagnostic)
	}
	fields := make(map[string]string)
	for _, violation := range diagnostic.FieldViolations {
		fields[violation.Path] = violation.Actual
	}
	if fields["workspaceManifest.revision"] != manifest.Revision || fields["preparation.targetRef"] != preparation.TargetRef || fields["nativeCheckRef"] != check {
		t.Fatalf("preparation identities were conflated: %v", fields)
	}
	writeNativeStructureSource(t, workspace, "helpers_test.go", "package fixture\nimport \"testing\"\nfunc verify(t *testing.T) { if 1 != 1 { t.Fatal(\"bad\") } }\n")
	manifest.TargetRefs = append(manifest.TargetRefs, "helpers_test.go")
	manifest.Revision = "tree-helper-ready"
	preparation, ready, supported = shellValidationProofPreparation(workspace, manifest, "catalog-after", true, recipe, requirement, attestations)
	if !ready || !supported || preparation.TargetRef != "" || preparation.OriginalDiagnostic != nil {
		t.Fatalf("resolved helper requested another acquisition: %+v ready=%t supported=%t", preparation, ready, supported)
	}
}

func TestNativeCheckDiagnosticBoundsTargetsWithoutExportingSource(t *testing.T) {
	requirement := shellValidationProofRequirementForTest(t, "", "cwi-ambiguous")
	var refs []string
	for index := 0; index <= runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs; index++ {
		refs = append(refs, fmt.Sprintf("checks/check-%02d_test.go", index))
	}
	diagnostic := shellNativeCheckReadinessDiagnostic(shellNativeCheckInspection{Code: "native_check_ambiguous", CheckRef: "TestWorkOrchCWI_A", TargetRefs: refs}, requirement, runtimecontract.WorkspaceManifest{Revision: "tree"}, "catalog", "checks/new_test.go")
	if err := diagnostic.Validate(); err != nil {
		t.Fatal(err)
	}
	if !diagnostic.Truncated || diagnostic.OmittedReason == "" || len(diagnostic.TargetRefs) != runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs {
		t.Fatalf("unbounded diagnostic: %+v", diagnostic)
	}
}
