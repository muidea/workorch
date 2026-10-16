package shell

import (
	"reflect"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellV3CompositionPublishesOriginalDeclarationsWithoutExecution(t *testing.T) {
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectConjunction
	recipe.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocolV3
	item := runtimecontract.RuntimeCoverageItem{ID: requirement.CoverageWorkItemID, SourceScope: requirement.SourceScope, SourceID: requirement.SourceID, TargetText: "original criterion"}
	rules := []runtimecontract.RuntimeLineageAssertionRule{{Ref: "delivery", Kind: runtimecontract.RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: runtimecontract.RuntimeProofPhasePostTerminalDelivery}}
	var err error
	requirement.LineageAssertions, err = runtimecontract.CompileRuntimeLineageAssertions(rules, requirement.DeclarationRef, item)
	if err != nil {
		t.Fatal(err)
	}
	requirement.Hash = runtimecontract.StableRuntimeValidationProofRequirementHash(requirement)
	checks, supported := shellValidationCheckAttestations(recipe, requirement)
	if !supported {
		t.Fatal("v3 project adapter unavailable")
	}
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	directory := t.TempDir()
	writeNativeStructureSource(t, directory, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, directory, target, conjunctiveSourceInput(checks, requirement.SourceObligationChecks[1].CheckRef, `if delivery.Compute(4)!=11 {t.Fatal("B")}`))
	manifest := shellTestWorkspaceManifestBinding("composition", []string{"go.mod", target})
	surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationProofRequirement{requirement}, manifest, "policy", directory)
	if len(surfaces) != 1 || len(surfaces[0].ProofRequirementBindings) != 1 {
		t.Fatalf("missing original subject: %+v", surfaces)
	}
	binding := surfaces[0].ProofRequirementBindings[0]
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
	if binding.InputReview.ProtocolRef != runtimecontract.RuntimeValidationInputReviewProtocolV3 || !reflect.DeepEqual(binding.InputReview.LineageAssertions, requirement.LineageAssertions) {
		t.Fatal("owner altered Definition consumers")
	}
	binding.InputReview.LineageAssertions[0].SourceTextHash = "changed"
	if surfaces[0].ProofRequirementBindings[0].InputReview.LineageAssertions[0].SourceTextHash == requirement.LineageAssertions[0].SourceTextHash {
		t.Fatal("test did not mutate owner subject")
	}
	if requirement.LineageAssertions[0].SourceTextHash == "changed" {
		t.Fatal("owner leaked caller declaration slice")
	}
	for _, adapter := range []string{modulecommon.ValidationRecipeAdapterIsolatedEntrypoint, modulecommon.ValidationRecipeAdapterConjunctiveEntrypoint} {
		unsupported := recipe
		unsupported.AdapterRef = adapter
		if len(resolveRuntimeValidationRecipeDeclarations([]runtimecontract.RuntimeValidationRecipe{unsupported}, manifest.Manifest)) != 0 {
			t.Fatal("v3 silently widened unsupported adapter")
		}
	}
}
