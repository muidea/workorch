package llm

import (
	"encoding/json"
	"strings"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInputRevisionPurposeRetainsOwnerObservationMethodAndDeliveryWork(t *testing.T) {
	v := inputRevisionProviderFixture(t)
	v.Binding.InputReview.ScopeContractRef = "owner/review-scope"
	v.Binding.InputReview.ScopeRequirements = "Reviewed tests may execute a freshly built delivery process with owner instrumentation and the original shared deadline."
	v.Binding.InputReview.Hash = rc.StableRuntimeValidationInputReviewSubjectHash(*v.Binding.InputReview, v.Binding)
	v.Report.SubjectHash = v.Binding.InputReview.Hash
	r := runtimeSemanticWorkProviderRequestForTest(t, true, v)
	p, err := runtimeSemanticPlanProviderRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	var a runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(p.PromptContext.Sections[3].Content), &a); err != nil {
		t.Fatal(err)
	}
	got := a.Manifest.ImplementationPurpose.InputRevision.Binding.InputReview
	if got.ScopeRequirements != v.Binding.InputReview.ScopeRequirements || got.ScopeContractRef != v.Binding.InputReview.ScopeContractRef {
		t.Fatal("purpose lost the original owner's legal observation method")
	}
	for _, rule := range []string{"create/correct required delivery dependencies", "does not confine delivery work", "current edit targets do not define execution permissions"} {
		if !strings.Contains(a.OutputInstruction, rule) {
			t.Fatalf("purpose still confines dependency work or guesses execution authority: %s", rule)
		}
	}
	got.ScopeRequirements += " altered"
	raw, _ := json.Marshal(a)
	p.PromptContext.Sections[3].Content = string(raw)
	if validateRuntimeSemanticContextIntegrity(r.SemanticContext, r.SemanticManifest, 0, nil, nil, p.ContextBudget, p.PromptContext) == nil {
		t.Fatal("rendered owner method tampering was accepted")
	}
}
