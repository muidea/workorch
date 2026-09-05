package llm

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPrepareRuntimeSemanticPlanRequestRejectsRawActivationAuthorityBeforeClone(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	policy := runtimecontract.RuntimeEvidencePolicy{
		ID: "semantic-preflight-raw-authority", AppliesTo: runtimecontract.StepKindShellCommand,
		CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
			ID: "semantic-preflight-proof", Scope: "dodItem",
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			ValidationRecipeID:    "semantic-preflight-recipe",
			RequiredCheckRefs:     []string{"semantic-preflight-check"},
		}},
	}.Normalized()
	policy.CoverageRequirements[0].RequiredCheckRefs = append(
		policy.CoverageRequirements[0].RequiredCheckRefs,
		policy.CoverageRequirements[0].RequiredCheckRefs[0],
	)
	request.SemanticContext.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}

	if _, err := prepareRuntimeSemanticPlanRequest(request); err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("semantic provider preparation accepted noncanonical authority: %v", err)
	}
}
