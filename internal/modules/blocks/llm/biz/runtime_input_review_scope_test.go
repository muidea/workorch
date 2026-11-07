package llm

import (
	"encoding/json"
	"strings"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInputReviewSemanticScopeIsCompleteInC3(t *testing.T) {
	request := runtimeInputReviewProviderFixture(t)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	catalog := &request.SemanticManifest.OwnerCatalogs[0]
	binding := &catalog.ValidationSurfaces[0].ProofRequirementBindings[0]
	binding.InputReview.EvaluationScope = rc.RuntimeValidationInputSemantics
	binding.InputReview.Hash = rc.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, *binding)
	catalog.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	request.SemanticManifest.Hash = rc.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	refreshSemanticProgressForTest(t, &request)
	current, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 2; i++ {
		if before.PromptContext.Sections[i].Content != current.PromptContext.Sections[i].Content {
			t.Fatalf("scope changed stable C%d", i)
		}
	}
	var projection runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(current.PromptContext.Sections[3].Content), &projection); err != nil {
		t.Fatal(err)
	}
	review := &projection.Manifest.ProofObligations[0].ValidationProofRequirement.InputReviews[0]
	if review.EvaluationScope != rc.RuntimeValidationInputSemantics || !strings.Contains(projection.OutputInstruction, "uncertaintyKind=semantic_information_missing") || !strings.Contains(projection.OutputInstruction, "aggregate elapsed time") {
		t.Fatal("declared stage or complete instructions missing")
	}
	review.EvaluationScope = ""
	corrupt, _ := json.Marshal(projection)
	current.PromptContext.Sections[3].Content = string(corrupt)
	if validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, current.ContextBudget, current.PromptContext) == nil {
		t.Fatal("lost stage boundary admitted by rendered integrity gate")
	}
}
