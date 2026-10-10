package definitionadmission

import (
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSpecificationVerificationApplicabilityRequiresEveryAfterImageSourceAndRetainedContribution(t *testing.T) {
	first := "dod:formal-source:item:000001"
	second := "dod:formal-source:item:000002"
	items := []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: second}, {CandidateItemRef: first}}
	spec := workspacemodel.GoalSpecification{Obligations: []workspacemodel.GoalSpecificationObligation{{ID: "obligation", DoDItemRefs: []string{first, second}}}}
	verification := workspacemodel.GoalSpecificationVerification{ID: "v", ObligationID: "obligation", Status: workspacemodel.GoalVerificationDeclared, ProfileID: "exact", EvidenceRequirementID: "dod-001", Scope: "dodItem"}
	profile := runtimecontract.RuntimeProfileContract{ID: "exact", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
		ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}}}
	if SpecificationVerificationApplies(spec, verification, items, []runtimecontract.RuntimeProfileContract{profile}) {
		t.Fatal("one exact source declaration incorrectly applied to two DoD sources")
	}

	profile.EvidenceRequirements[0].ID = "*"
	verification.ProfileID = "wildcard"
	verification.EvidenceRequirementID = "*"
	profile.ID = "wildcard"
	profiles := []runtimecontract.RuntimeProfileContract{profile}
	if !SpecificationVerificationApplies(spec, verification, items, profiles) {
		t.Fatal("wildcard policy did not apply to every formal DoD source")
	}

	wrongScope := runtimecontract.CloneRuntimeProfileContracts(profiles)
	wrongScope[0].EvidenceRequirements[0].Scope = "deliverable"
	verification.Scope = "deliverable"
	if SpecificationVerificationApplies(spec, verification, items, wrongScope) {
		t.Fatal("wrong source scope was accepted")
	}
}

func TestSpecificationVerificationApplicabilityRejectsUnknownOrInvalidSourceGraph(t *testing.T) {
	spec := workspacemodel.GoalSpecification{Obligations: []workspacemodel.GoalSpecificationObligation{{ID: "obligation", DoDItemRefs: []string{"dod:formal-source:item:000001"}}}}
	verification := workspacemodel.GoalSpecificationVerification{ObligationID: "obligation", ProfileID: "p", EvidenceRequirementID: "*", Scope: "dodItem"}
	profile := runtimecontract.RuntimeProfileContract{ID: "p", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}}}}
	for _, ref := range []string{"dod:formal-source:item:000002", "derived-dod:unknown:000001", "resource:chapter:000001"} {
		items := []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: ref}}
		if SpecificationVerificationApplies(spec, verification, items, []runtimecontract.RuntimeProfileContract{profile}) {
			t.Fatalf("invalid/mismatched source graph accepted ref %q", ref)
		}
	}
}

func TestSpecificationVerificationApplicabilityRejectsConflictingMergedPolicy(t *testing.T) {
	ref := "dod:formal-source:item:000001"
	items := []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: ref}}
	spec := workspacemodel.GoalSpecification{Obligations: []workspacemodel.GoalSpecificationObligation{{ID: "obligation", DoDItemRefs: []string{ref}}}}
	verification := workspacemodel.GoalSpecificationVerification{ObligationID: "obligation", ProfileID: "exact", EvidenceRequirementID: "dod-001", Scope: "dodItem"}
	profiles := []runtimecontract.RuntimeProfileContract{
		{ID: "exact", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, VerificationPolicy: &runtimecontract.RuntimeVerificationPolicy{Version: runtimecontract.RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 2}}}},
		{ID: "wildcard", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, VerificationPolicy: &runtimecontract.RuntimeVerificationPolicy{Version: runtimecontract.RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 4}}}},
	}
	if SpecificationVerificationApplies(spec, verification, items, profiles) {
		t.Fatal("conflicting exact/wildcard policy contributions were accepted")
	}
}

func TestValidateAuthoringVerificationApplicabilityAllowsOnlyUnboundUnverified(t *testing.T) {
	items := []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: "derived-dod:user:000001"}}
	spec := workspacemodel.GoalSpecification{Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "v", Status: workspacemodel.GoalVerificationUnverified}}}
	if err := ValidateAuthoringVerificationApplicability(spec, items, nil); err != nil {
		t.Fatalf("unbound UNVERIFIED should represent a gap: %v", err)
	}
	spec.Verifications[0].ProfileID = "self-authored"
	if err := ValidateAuthoringVerificationApplicability(spec, items, nil); err == nil || !strings.Contains(err.Error(), "verification/v") {
		t.Fatalf("bound UNVERIFIED error = %v", err)
	}
}
