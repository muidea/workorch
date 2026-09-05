package runtime

import (
	"context"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticPlanRequestRejectsRawActivationAuthorityBeforeProvider(t *testing.T) {
	request := semanticBoundaryRequest(t)
	policy := runtimecontract.RuntimeEvidencePolicy{
		ID: "semantic-raw-authority", AppliesTo: runtimecontract.StepKindShellCommand,
		CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
			ID: "semantic-proof", Scope: "dodItem",
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			ValidationRecipeID:    "semantic-recipe",
			RequiredCheckRefs:     []string{"semantic-check"},
		}},
	}.Normalized()
	policy.CoverageRequirements[0].RequiredCheckRefs = append(
		policy.CoverageRequirements[0].RequiredCheckRefs,
		policy.CoverageRequirements[0].RequiredCheckRefs[0],
	)
	request.SemanticContext.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}

	providerCalls := 0
	_, err := RequestCandidate(context.Background(), candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		providerCalls++
		return generationResult([]byte(`{"documentKind":"semantic_plan"}`), nil)
	}), request)
	if err == nil || !strings.Contains(err.Error(), "evidence polic") || providerCalls != 0 {
		t.Fatalf("noncanonical semantic authority reached provider: calls=%d err=%v", providerCalls, err)
	}
}
