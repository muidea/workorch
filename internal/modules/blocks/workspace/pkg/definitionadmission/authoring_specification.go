package definitionadmission

import (
	"fmt"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ValidateAuthoringVerificationApplicability 只检查声明适用性，不生成准入、owner fact、binding、proof 或 credit。
// 无绑定 UNVERIFIED 可以表达缺口；确认和正式准入仍须独立阻断该缺口。
func ValidateAuthoringVerificationApplicability(spec workspacemodel.GoalSpecification, dodItems []workspacemodel.DefinitionCandidateItem, profiles []runtimecontract.RuntimeProfileContract) error {
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
	return nil
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
