package workorchruntimecontract

import (
	"reflect"
	"strings"
	"testing"
)

func semanticSourceObligationReviewFixture(t *testing.T) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument) {
	t.Helper()
	manifest, document, oldBinding := semanticInputReviewV2Fixture(t)
	item := &manifest.CoverageItems[0]
	for _, id := range []string{"A", "B"} {
		check := RuntimeSourceObligationCheck{InventoryHash: "inventory", ObligationID: id, RequirementID: "requirement", VerificationID: "verify-" + id, Criterion: " " + id + " 独立原判据\n ", Declaration: "验证 " + id}
		check.CheckRef = StableRuntimeSourceObligationCheckRef(manifest.DefinitionRevision, item.ID, check)
		item.SourceObligationChecks = append(item.SourceObligationChecks, check)
	}
	requirement := runtimeValidationProofRequirementForTest(t, manifest.DefinitionRevision, *item, "", oldBinding.ValidationRecipeID, nil)
	binding, err := RuntimeValidationProofRequirementBindingForRecipe(requirement, oldBinding.ValidationRecipeID, runtimeValidationCheckAttestationsForTest(requirement.RequiredCheckRefs)...)
	if err != nil {
		t.Fatal(err)
	}
	input := oldBinding.InputReview.Clone()
	binding.InputReview = &input
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(input, binding)
	for i := range manifest.OwnerCatalogs {
		catalog := &manifest.OwnerCatalogs[i]
		if catalog.OwnerModuleID != RuntimeModuleShell {
			continue
		}
		catalog.ValidationSurfaces[0].CheckRefs = requirement.RequiredCheckRefs
		catalog.ValidationSurfaces[0].ProofRequirementBindings = []RuntimeValidationProofRequirementBinding{binding}
		catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	}
	for i := range manifest.ProofObligations {
		if manifest.ProofObligations[i].CoverageWorkItemID == item.ID {
			manifest.ProofObligations[i].ValidationProofRequirement = &requirement
		}
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	document.ManifestHash = manifest.Hash
	review := &RuntimeValidationInputReview{SubjectHash: binding.InputReview.Hash}
	for _, ref := range requirement.RequiredCheckRefs {
		criterion := item.TargetText
		for _, source := range item.SourceObligationChecks {
			if source.CheckRef == ref {
				criterion = source.Criterion
			}
		}
		review.Checks = append(review.Checks, RuntimeValidationInputCheckReview{RequiredCheckRef: ref, Conclusion: RuntimeValidationInputConsistent, Reason: "完整独立判据有实际断言", InputTargetRefs: []string{input.Files[0].TargetRef},
			Obligations: []RuntimeValidationInputObligationReview{{CriterionText: strings.TrimSpace(criterion), Finding: RuntimeValidationInputCovered, Reason: "输入断言覆盖全部原判据", InputTargetRefs: []string{input.Files[0].TargetRef}}}})
	}
	document.Proposals[1].ValidationInputReview = review
	return manifest, document
}

func TestRuntimeSourceObligationReviewRequiresEachCheckOwnCriterion(t *testing.T) {
	manifest, document := semanticSourceObligationReviewFixture(t)
	before := cloneRuntimeSemanticPlanDocumentRaw(document)
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	payload, err := EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, payload)
	if err != nil || !reflect.DeepEqual(decoded.Normalized(), document.Normalized()) {
		t.Fatalf("exact source review round trip: %v", err)
	}
	for _, replacement := range []string{manifest.CoverageItems[0].TargetText, strings.TrimSpace(manifest.CoverageItems[0].SourceObligationChecks[0].Criterion), "B 独立"} {
		bad := cloneRuntimeSemanticPlanDocumentRaw(document)
		for i := range bad.Proposals[1].ValidationInputReview.Checks {
			check := &bad.Proposals[1].ValidationInputReview.Checks[i]
			if check.RequiredCheckRef == manifest.CoverageItems[0].SourceObligationChecks[1].CheckRef {
				check.Obligations[0].CriterionText = replacement
			}
		}
		if bad.ValidateAgainstManifest(manifest) == nil {
			t.Fatalf("B review accepted substituted or incomplete criterion %q", replacement)
		}
	}
	if !reflect.DeepEqual(document, before) {
		t.Fatal("source review admission mutated original bytes")
	}
}
