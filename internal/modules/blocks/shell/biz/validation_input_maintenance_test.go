package shell

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProjectAdditionalTestsShareInventoryAndReadinessRules(t *testing.T) {
	recipe, r, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	checks, ok := shellValidationCheckAttestations(recipe, r)
	if !ok {
		t.Fatal("mapping")
	}
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(r, checks...)
	source := "package checks\nimport \"testing\"\nfunc " + checks[0].NativeCheckRef + "(t *testing.T){t.Fatal(\"real check\")}\ntype helper struct{}\nfunc(helper) TestMethod(){}\nfunc Testhelper(){}\nfunc TestExtra(t *testing.T){t.Fatal(\"additive failure\")}\n"
	for _, check := range checks[1:] {
		source += "func " + check.NativeCheckRef + "(t *testing.T){t.Fatal(\"required\")}\n"
	}
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, target, source)
	refs := []string{"go.mod", target}
	inspection := inspectShellNativeValidationChecks(root, refs, checks)
	if inspection.Code != "" {
		t.Fatalf("legal additive form rejected: %+v", inspection)
	}
	b, err := rc.RuntimeValidationProofRequirementBindingForRecipe(r, recipe.ID, checks...)
	if err != nil {
		t.Fatal(err)
	}
	subject, code := shellInputReview(root, refs, b)
	if code != "" || len(subject.DiagnosticSources) != len(checks)+1 {
		t.Fatalf("inventory differs: %s %+v", code, subject)
	}
	for _, suffix := range []string{"func init(){}", "func TestMain(m *testing.M){}"} {
		file, err := parser.ParseFile(token.NewFileSet(), target, source+suffix, 0)
		if err != nil {
			t.Fatal(err)
		}
		if shellConjunctiveInputDeclarations(file, checks) {
			t.Fatal("runner interception admitted")
		}
	}
	// The original required native check is still mandatory; the additive test
	// cannot substitute its identity or assertions.
	writeNativeStructureSource(t, root, target, strings.Replace(source, checks[0].NativeCheckRef, "helperOriginal", 1))
	if got := inspectShellNativeValidationChecks(root, refs, checks); got.Code != "native_check_missing" {
		t.Fatalf("lost required member: %+v", got)
	}
	requirements := shellProjectContentRequirements(recipe.AdapterRef)
	if strings.Contains(requirements, "16 files/64 KiB") || strings.Contains(requirements, "未声明 native Test") || !strings.Contains(requirements, "64 files/256 KiB") {
		t.Fatal("selected instructions contradict reader")
	}
}

func TestStructuralMaintenanceRetainsExactMalformedInputWithoutReview(t *testing.T) {
	recipe, r, checks := conjunctiveSourceRequirement(t)
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(r, checks...)
	source := conjunctiveSourceInput(checks, "", "") + "func init() {}\n"
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, target, source)
	refs := []string{"go.mod", target}
	manifest := shellTestWorkspaceManifestBinding("maintenance", refs)
	surfaces := shellValidationSurfaces([]rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationProofRequirement{r}, manifest, "policy", root)
	if len(surfaces) != 1 || len(surfaces[0].ProofRequirementUnavailabilities) != 1 {
		t.Fatalf("missing exact failure: %+v", surfaces)
	}
	u := surfaces[0].ProofRequirementUnavailabilities[0]
	if u.Maintenance == nil || u.Validate() != nil || len(u.Maintenance.TargetRefs) != 1 || u.Maintenance.TargetRefs[0] != target || u.Maintenance.Binding.InputReview != nil || len(surfaces[0].ProofRequirementBindings) != 0 {
		t.Fatalf("unbound or falsely ready maintenance: %+v", u)
	}
	found := false
	for _, f := range u.Maintenance.Files {
		if f.TargetRef == target {
			found = f.Content == source
		}
	}
	if !found {
		t.Fatal("original malformed bytes were changed")
	}
	for _, code := range []string{"native_check_analysis_limit", "native_check_source_out_of_scope", "native_check_protocol_unsupported", "native_check_source_unavailable"} {
		if shellValidationInputMaintenance(root, refs, shellNativeCheckInspection{Code: code, TargetRefs: []string{target}}, r, recipe, checks) != nil {
			t.Fatal("unavailable failure acquired authority", code)
		}
	}
}
