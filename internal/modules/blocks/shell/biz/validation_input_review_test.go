package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestVersionedInputReviewCompleteBoundedSourceAndFreshness(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "native", Command: "go", Args: []string{"test", "./..."}, AdapterRef: modulecommon.ValidationRecipeAdapterVersionedEntrypoint}
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-review")
	checks, _ := shellValidationCheckAttestations(recipe, requirement)
	binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	input := fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T){ if false { t.Fatal(\"assertion\") } }", checks[0].NativeCheckRef)
	writeNativeStructureSource(t, root, target, input)
	writeNativeStructureSource(t, root, "workorch_validation/v3/helper.go", "package checks\n")
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, "go.sum", "") // a complete empty auxiliary file is not missing context
	refs := []string{target, "workorch_validation/v3/helper.go", "go.mod", "go.sum"}
	subject, code := shellVersionedInputReview(root, refs, binding)
	if code != "" || len(subject.Files) != 4 {
		t.Fatalf("complete source: code=%s subject=%+v", code, subject)
	}
	binding.InputReview = subject
	// Modify after reading: the observation must retain the reviewed bytes,
	// allowing the existing freshness guard to reject the newer target.
	writeNativeStructureSource(t, root, target, input+"\n// changed")
	inventory := runtimecontract.RuntimeOperationAdmissibility{ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{ProofRequirementBindings: []runtimecontract.RuntimeValidationProofRequirementBinding{binding}}}}
	observation := NewRuntimeAdapter().runtimeAdmissibilityObservation(root, "review", inventory, refs)
	found := false
	for _, dependency := range observation.Dependencies {
		if dependency.SubjectRef == target && dependency.Revision == runtimecontract.RuntimeValidationInputContentHash(input) {
			found = true
		}
	}
	if !found {
		t.Fatal("freshness observation blessed bytes not reviewed")
	}
	changed, _ := shellVersionedInputReview(root, refs, binding)
	if changed.Hash == subject.Hash {
		t.Fatal("content drift reused prior review identity")
	}
	writeNativeStructureSource(t, root, "workorch_validation/v3/helper.go", strings.Repeat("a", runtimecontract.RuntimeValidationInputReviewMaxBytes))
	if _, code := shellVersionedInputReview(root, refs, binding); code != "validation_input_review_capacity_exceeded" {
		t.Fatalf("truncation admitted: %s", code)
	}
	if err := os.Symlink(filepath.Join(root, target), filepath.Join(root, "workorch_validation/v3/link.go")); err != nil {
		t.Fatal(err)
	}
	if _, code := shellVersionedInputReview(root, []string{target, "workorch_validation/v3/link.go"}, binding); code != "validation_input_review_source_unavailable" {
		t.Fatalf("symlink admitted: %s", code)
	}
}

func TestInputReviewProtocolSelectionPreservesOriginalInputsAndNativeMapping(t *testing.T) {
	root, recipe := t.TempDir(), isolatedInputRecipe()
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-review-version")
	checks, _ := shellValidationCheckAttestations(recipe, requirement)
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	input := fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T){ if false { t.Fatal(\"not proof\") } }", checks[0].NativeCheckRef)
	writeNativeStructureSource(t, root, target, input)
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	manifest := shellTestWorkspaceManifestBinding("exact-tree", []string{"go.mod", target})
	acquire := func(recipe runtimecontract.RuntimeValidationRecipe) runtimecontract.RuntimeValidationProofRequirementBinding {
		t.Helper()
		recipes := []runtimecontract.RuntimeValidationRecipe{recipe}
		surfaces := shellValidationSurfaces(recipes, recipes, recipes, []runtimecontract.RuntimeValidationProofRequirement{requirement}, manifest, "policy", root)
		if len(surfaces) != 1 || len(surfaces[0].ProofRequirementBindings) != 1 || surfaces[0].Validate() != nil {
			t.Fatal("missing exact input review observation")
		}
		return surfaces[0].ProofRequirementBindings[0]
	}
	legacy := acquire(recipe)
	updated := recipe
	updated.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocolV2
	current := acquire(updated)
	if legacy.InputReview.ProtocolRef != runtimecontract.RuntimeValidationInputReviewProtocol || current.InputReview.ProtocolRef != updated.InputReviewProtocolRef || legacy.InputReview.Hash == current.InputReview.Hash || shellValidationRecipeSchemaHash(recipe) == shellValidationRecipeSchemaHash(updated) {
		t.Fatal("protocol selection did not require new sealed identities")
	}
	if !reflect.DeepEqual(legacy.CheckAttestations, current.CheckAttestations) || !reflect.DeepEqual(legacy.InputReview.Files, current.InputReview.Files) || legacy.RequirementHash != current.RequirementHash {
		t.Fatal("new review protocol replaced the original inputs or criterion")
	}
	if repeated := acquire(recipe); !reflect.DeepEqual(legacy, repeated) {
		t.Fatal("current protocol was hot-loaded into an old declaration")
	}
	unsupported := updated
	unsupported.AdapterRef = modulecommon.ValidationRecipeAdapterProjectNative
	if got := resolveRuntimeValidationRecipeDeclarations([]runtimecontract.RuntimeValidationRecipe{unsupported}, manifest.Manifest); len(got) != 0 {
		t.Fatal("owner claimed an unsupported protocol/adapter combination")
	}
	content, err := os.ReadFile(filepath.Join(root, target))
	if err != nil || string(content) != input {
		t.Fatal("read-only protocol selection changed the original input")
	}
}
