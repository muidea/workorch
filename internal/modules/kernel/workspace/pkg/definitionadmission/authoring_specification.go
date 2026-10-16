package definitionadmission

import (
	"fmt"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ValidateAuthoringVerificationApplicability 只检查声明适用性，不生成准入、owner fact、binding、proof 或 credit。
// 无绑定 UNVERIFIED 可以表达缺口；确认和正式准入仍须独立阻断该缺口。
func ValidateAuthoringVerificationApplicability(spec workspacemodel.GoalSpecification, dodItems []workspacemodel.DefinitionCandidateItem, profiles []runtimecontract.RuntimeProfileContract, goals ...workspacemodel.DefinitionCandidateItem) error {
	for _, verification := range spec.Verifications {
		switch verification.Status {
		case workspacemodel.GoalVerificationUnverified:
			if verification.ProfileID != "" || verification.EvidenceRequirementID != "" || verification.Scope != "" {
				return fmt.Errorf("verification/%s: UNVERIFIED cannot carry Profile binding", verification.ID)
			}
		case workspacemodel.GoalVerificationDeclared:
			if !SpecificationVerificationApplies(spec, verification, dodItems, profiles) {
				return fmt.Errorf("verification/%s: declared Profile evidence is not applicable to every DoD source", verification.ID)
			}
		default:
			return fmt.Errorf("verification/%s: unknown verification status", verification.ID)
		}
	}
	return ValidateSpecificationExecutionProofMappings(spec, dodItems, profiles, goals...)
}

// Check the source graph before confirmation/admission, using the same exact
// post-confirmation DoD order resolver. No Runtime CWI IDs are accepted here.
func ValidateSpecificationExecutionProofMappings(spec workspacemodel.GoalSpecification, dodItems []workspacemodel.DefinitionCandidateItem, profiles []runtimecontract.RuntimeProfileContract, goals ...workspacemodel.DefinitionCandidateItem) error {
	resolved, ok := specificationDoDSources(dodItems)
	if !ok {
		return fmt.Errorf("execution proof mapping requires exact DoD source order")
	}
	inventory := runtimecontract.RuntimeSourceObligationInventory{Sources: []runtimecontract.RuntimeDefinitionSource{{CandidateItemRef: spec.GoalItemRef, SourceScope: "goal", SourceID: spec.GoalItemRef}}}
	if len(goals) == 1 {
		if goals[0].CandidateItemRef != spec.GoalItemRef || workspacemodel.StableDefinitionTextHash(goals[0].Text) != spec.GoalTextHash {
			return fmt.Errorf("execution source mapping Goal preimage mismatch")
		}
		inventory.Sources[0].Text = goals[0].Text
	} else if len(goals) > 1 {
		return fmt.Errorf("ambiguous Goal source preimage")
	}
	for _, item := range dodItems {
		inventory.Sources = append(inventory.Sources, runtimecontract.RuntimeDefinitionSource{CandidateItemRef: item.CandidateItemRef, SourceScope: "dodItem", SourceID: resolved[item.CandidateItemRef], Text: item.Text})
	}
	for _, mapping := range spec.ExecutionProofMappings {
		inventory.ExecutionProofMappings = append(inventory.ExecutionProofMappings, runtimecontract.RuntimeExecutionProofMapping(mapping))
	}
	for _, mapping := range spec.EvidencePurposeMappings {
		inventory.EvidencePurposeMappings = append(inventory.EvidencePurposeMappings, runtimecontract.RuntimeEvidencePurposeMapping(mapping))
	}
	if err := runtimecontract.ValidateRuntimeEvidencePurposeMappings(inventory, profiles); err != nil {
		return err
	}
	return runtimecontract.ValidateRuntimeExecutionProofMappings(inventory, profiles)
}

// SpecificationVerificationApplies 复用 Workspace 的确认后来源映射与中立 Profile matcher。
func SpecificationVerificationApplies(spec workspacemodel.GoalSpecification, verification workspacemodel.GoalSpecificationVerification, dodItems []workspacemodel.DefinitionCandidateItem, profiles []runtimecontract.RuntimeProfileContract) bool {
	sources, ok := specificationDoDSources(dodItems)
	if !ok {
		return false
	}
	var refs []string
	for _, obligation := range spec.Obligations {
		if obligation.ID == verification.ObligationID {
			refs = obligation.DoDItemRefs
			break
		}
	}
	if len(refs) == 0 {
		return false
	}
	var declaration *runtimecontract.RuntimeProfileEvidenceRequirement
	for _, profile := range profiles {
		if profile.ID != verification.ProfileID {
			continue
		}
		for _, value := range profile.EvidenceRequirements {
			if value.ID != verification.EvidenceRequirementID || value.Scope != verification.Scope {
				continue
			}
			if declaration != nil {
				return false
			}
			value.TargetSelectorRef, value.TargetSelectorOwnerRef, value.TargetSelectorSubjectRef = runtimecontract.ResolveRuntimeProfileTargetSelectorIdentity(profile.ID, value)
			declaration = &value
		}
	}
	if declaration == nil {
		return false
	}
	for _, ref := range refs {
		sourceID, exists := sources[ref]
		if !exists || !runtimecontract.RuntimeEvidenceDeclarationMatchesSource(*declaration, "dodItem", sourceID) {
			return false
		}
		policy, err := runtimecontract.CompileRuntimeSourceEvidencePolicy("dodItem", sourceID, profiles, nil)
		if err != nil || !runtimecontract.RuntimeSourceEvidencePolicyRetainsDeclaration(policy, *declaration, "dodItem", sourceID) {
			return false
		}
	}
	return true
}
