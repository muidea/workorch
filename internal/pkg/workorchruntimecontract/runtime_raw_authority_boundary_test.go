package workorchruntimecontract

import (
	"strings"
	"testing"
)

func rawAuthorityEvidencePolicyForBoundaryTest() RuntimeEvidencePolicy {
	return RuntimeEvidencePolicy{
		ID: "raw-authority-policy", AppliesTo: StepKindShellCommand,
		CoverageRequirements: []RuntimeCoverageEvidenceRequirement{{
			ID: "raw-authority-requirement", Scope: "dodItem",
			RequiredEvidenceKinds: []string{EvidenceKindValidation},
			ValidationRecipeID:    "raw-authority-recipe",
			RequiredCheckRefs:     []string{"raw-authority-check"},
		}},
	}.Normalized()
}

func nonCanonicalRawAuthorityEvidencePoliciesForBoundaryTest() []RuntimeEvidencePolicy {
	policy := rawAuthorityEvidencePolicyForBoundaryTest()
	policy.CoverageRequirements[0].RequiredCheckRefs = append(
		policy.CoverageRequirements[0].RequiredCheckRefs,
		policy.CoverageRequirements[0].RequiredCheckRefs[0],
	)
	return []RuntimeEvidencePolicy{policy}
}

func TestRuntimeCandidateContextRejectsRawActivationAuthorityBeforeNormalization(t *testing.T) {
	context := RuntimeCandidateContext{
		Activation: RuntimeActivationContext{EvidencePolicies: nonCanonicalRawAuthorityEvidencePoliciesForBoundaryTest()},
	}
	if err := context.Validate(RuntimeDecision{}); err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("candidate context accepted noncanonical activation authority: %v", err)
	}
}

func TestRuntimeSemanticPlanRequestContextPreservesAndRejectsRawActivationAuthority(t *testing.T) {
	context := RuntimeSemanticPlanRequestContext{
		Activation: RuntimeActivationContext{EvidencePolicies: nonCanonicalRawAuthorityEvidencePoliciesForBoundaryTest()},
	}
	cloned := context.Clone()
	if len(cloned.Activation.EvidencePolicies) != 1 || len(cloned.Activation.EvidencePolicies[0].CoverageRequirements) != 1 ||
		len(cloned.Activation.EvidencePolicies[0].CoverageRequirements[0].RequiredCheckRefs) != 2 {
		t.Fatalf("semantic context clone laundered activation authority: source=%+v clone=%+v", context.Activation.EvidencePolicies, cloned.Activation.EvidencePolicies)
	}
	if err := ValidateRuntimeEvidencePolicies(cloned.Activation.EvidencePolicies); err == nil {
		t.Fatalf("semantic context clone repaired invalid activation authority: %+v", cloned.Activation.EvidencePolicies)
	}
	if err := context.Validate(RuntimeSemanticPlanManifest{}); err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("semantic context accepted noncanonical activation authority: %v", err)
	}
}

func TestRuntimeOperationExecutionContextRejectsRawEvidenceAuthorityBeforeScope(t *testing.T) {
	context := RuntimeOperationExecutionContext{EvidencePolicies: nonCanonicalRawAuthorityEvidencePoliciesForBoundaryTest()}
	if err := ValidateRuntimeOperationExecutionContextScope(context); err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("execution context accepted noncanonical evidence authority: %v", err)
	}
}
