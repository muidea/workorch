package events

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func shellRawAuthorityPolicyForTest() runtimecontract.RuntimeEvidencePolicy {
	return runtimecontract.RuntimeEvidencePolicy{
		ID: "shell-raw-authority", AppliesTo: runtimecontract.StepKindShellCommand,
		CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
			ID: "shell-proof", Scope: "dodItem",
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			ValidationRecipeID:    "shell-proof-recipe",
			RequiredCheckRefs:     []string{"shell-proof-check"},
		}},
	}.Normalized()
}

func duplicateShellRawAuthorityCheckForTest(policies []runtimecontract.RuntimeEvidencePolicy) []runtimecontract.RuntimeEvidencePolicy {
	policies = runtimecontract.CloneRuntimeEvidencePolicies(policies)
	requirement := &policies[0].CoverageRequirements[0]
	requirement.RequiredCheckRefs = append(requirement.RequiredCheckRefs, requirement.RequiredCheckRefs[0])
	return policies
}

func TestShellSealAndExecutionRejectRawEvidenceAuthorityBeforeNormalization(t *testing.T) {
	operation, catalog, _, input, executionContext := shellSealFixture(t)
	input.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{shellRawAuthorityPolicyForTest()}

	invalidInput := input
	invalidInput.EvidencePolicies = duplicateShellRawAuthorityCheckForTest(input.EvidencePolicies)
	if _, err := SealRuntimeOperation(operation, catalog, invalidInput); err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("Shell seal accepted noncanonical evidence authority: %v", err)
	}

	sealed, err := SealRuntimeOperation(operation, catalog, input)
	if err != nil {
		t.Fatal(err)
	}
	sealed.Authorization.EvidencePolicies = duplicateShellRawAuthorityCheckForTest(sealed.Authorization.EvidencePolicies)
	if _, _, err := sealed.ValidateForExecution(executionContext); err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("Shell execution accepted noncanonical authorization authority: %v", err)
	}
}
