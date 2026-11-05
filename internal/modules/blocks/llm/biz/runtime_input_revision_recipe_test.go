package llm

import (
	"encoding/json"
	"reflect"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInputRevisionProjectionRetainsOriginalExecutionRecipe(t *testing.T) {
	v := inputRevisionProviderFixture(t)
	b, err := json.Marshal(inputRevisionForProvider(v))
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]json.RawMessage
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if len(p["Recipe"]) == 0 {
		t.Fatal("input maintenance lost the original selected recipe and its declared execution deadline")
	}
	var recipe = *v.Recipe
	if err := json.Unmarshal(p["Recipe"], &recipe); err != nil || !reflect.DeepEqual(recipe, *v.Recipe) {
		t.Fatal("projection substituted recipe metadata", err)
	}
}

func TestInputRevisionRecipeStaysInC3AndRejectsRenderedTampering(t *testing.T) {
	v := inputRevisionProviderFixture(t)
	v.Recipe.TimeoutMillis = 60000
	v.Binding.InputReview.Hash = rc.StableRuntimeValidationInputReviewSubjectHash(*v.Binding.InputReview, v.Binding)
	v.Report.SubjectHash = v.Binding.InputReview.Hash
	request := runtimeSemanticWorkProviderRequestForTest(t, true, v)
	before, err := runtimeSemanticPlanProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	v.Recipe.TimeoutMillis = 300000
	v.Binding.InputReview.Hash = rc.StableRuntimeValidationInputReviewSubjectHash(*v.Binding.InputReview, v.Binding)
	v.Report.SubjectHash = v.Binding.InputReview.Hash
	after, err := runtimeSemanticPlanProviderRequest(runtimeSemanticWorkProviderRequestForTest(t, true, v))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if before.PromptContext.Sections[i].Content != after.PromptContext.Sections[i].Content {
			t.Fatal("volatile input method drifted stable C0-C2")
		}
	}
	var authority runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(before.PromptContext.Sections[3].Content), &authority); err != nil {
		t.Fatal(err)
	}
	p := authority.Manifest.ImplementationPurpose.InputRevision
	if p.Recipe == nil || p.Recipe.TimeoutMillis != 60000 {
		t.Fatal("frozen old transaction used new method")
	}
	p.Recipe.TimeoutMillis = 300000
	raw, _ := json.Marshal(authority)
	before.PromptContext.Sections[3].Content = string(raw)
	if validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, before.ContextBudget, before.PromptContext) == nil {
		t.Fatal("rendered deadline substitution passed exact input integrity")
	}
	p = inputRevisionForProvider(v)
	p.Recipe.Args = append(p.Recipe.Args, "mutated")
	p.Recipe.TimeoutMillis = 1000
	if v.Recipe.TimeoutMillis != 300000 || reflect.DeepEqual(p.Recipe.Args, v.Recipe.Args) {
		t.Fatal("projection aliases owner method")
	}
}

func TestIndependentReviewReceivesExactDeclaredMethod(t *testing.T) {
	r := runtimeInputReviewProviderFixture(t)
	c := &r.SemanticManifest.OwnerCatalogs[0]
	b := &c.ValidationSurfaces[0].ProofRequirementBindings[0]
	method := c.ValidationRecipes[0]
	method.TimeoutMillis = 300000
	c.ValidationRecipes[0] = method
	b.InputReview.Recipe = &method
	b.InputReview.Hash = rc.StableRuntimeValidationInputReviewSubjectHash(*b.InputReview, *b)
	c.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(*c)
	r.SemanticManifest.Hash = rc.StableRuntimeSemanticPlanManifestHash(r.SemanticManifest)
	refreshSemanticProgressForTest(t, &r)
	p, err := runtimeSemanticPlanProviderRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	var authority runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(p.PromptContext.Sections[3].Content), &authority); err != nil {
		t.Fatal(err)
	}
	projection := &authority.Manifest.ProofObligations[0].ValidationProofRequirement.InputReviews[0]
	if !reflect.DeepEqual(projection.Recipe, &method) {
		t.Fatal("independent review lost exact owner method")
	}
	projection.Recipe.TimeoutMillis = 60000
	raw, _ := json.Marshal(authority)
	p.PromptContext.Sections[3].Content = string(raw)
	if validateRuntimeSemanticContextIntegrity(r.SemanticContext, r.SemanticManifest, 0, nil, nil, p.ContextBudget, p.PromptContext) == nil {
		t.Fatal("independent review accepted a substituted method")
	}
	if b.InputReview.Recipe.TimeoutMillis != 300000 {
		t.Fatal("review projection aliases owner source")
	}
}
