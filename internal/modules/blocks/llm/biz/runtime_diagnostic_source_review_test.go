package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeDiagnosticSourcesAreIndependentProtectedC3AndSchemaOnly(t *testing.T) {
	request := runtimeInputReviewProviderFixture(t)
	catalog := &request.SemanticManifest.OwnerCatalogs[0]
	binding := &catalog.ValidationSurfaces[0].ProofRequirementBindings[0]
	binding.InputReview.ProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocol
	binding.InputReview.DiagnosticSourceContractRef = runtimecontract.RuntimeValidationDiagnosticSourceReviewV1
	binding.InputReview.DiagnosticSources = []runtimecontract.RuntimeValidationDiagnosticSource{{SourceRef: "owner-source-a", NativeCaseRef: "owner/package/TestA", InputTargetRef: binding.InputReview.Files[0].TargetRef}, {SourceRef: "owner-source-b", NativeCaseRef: "owner/package/TestB", InputTargetRef: binding.InputReview.Files[0].TargetRef}}
	refresh := func() {
		binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, *binding)
		catalog.ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{{ID: binding.ValidationRecipeID, AdapterRef: "owner/source-review", Command: "check", InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol}}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
		request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
		refreshSemanticProgressForTest(t, &request)
	}
	refresh()
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	var projection runtimeSemanticAuthorityProjection
	if json.Unmarshal([]byte(before.PromptContext.Sections[3].Content), &projection) != nil {
		t.Fatal("C3 invalid")
	}
	input := projection.Manifest.ProofObligations[0].ValidationProofRequirement.InputReviews[0]
	if input.DiagnosticSourceContractRef != runtimecontract.RuntimeValidationDiagnosticSourceReviewV1 || !reflect.DeepEqual(input.DiagnosticSources, binding.InputReview.DiagnosticSources) || !strings.Contains(projection.OutputInstruction, "one independent report for EVERY exact sourceRef") {
		t.Fatal("source inventory/instruction omitted")
	}
	for _, field := range []string{"diagnosticSources", "sourceRef", "owner-source-a", "owner-source-b"} {
		if !strings.Contains(before.Schema, field) {
			t.Fatal("exact source schema missing", field)
		}
	}
	binding.InputReview.DiagnosticSources[0].NativeCaseRef = "owner/package/TestDifferent"
	refresh()
	after, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= 2; index++ {
		if before.PromptContext.Sections[index].Content != after.PromptContext.Sections[index].Content {
			t.Fatal("diagnostic source changed stable prefix", index)
		}
	}
	if before.ContextBudget.CachePlan.StablePrefixHash != after.ContextBudget.CachePlan.StablePrefixHash {
		t.Fatal("stable prefix drift")
	}
	if validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, before.ContextBudget, before.PromptContext) == nil {
		t.Fatal("stale exact source context accepted")
	}
	if err := validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, after.ContextBudget, after.PromptContext); err != nil {
		t.Fatal(err)
	}
}
