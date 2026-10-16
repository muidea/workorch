package workorchruntimecontract

import "fmt"

// ValidateRuntimeSourceObligationApplicability 在 bootstrap 与 fresh-load 重新核对
// 已封存的 Profile 声明适用性，不从声明存在推导 owner readiness、binding 或 credit。
func ValidateRuntimeSourceObligationApplicability(inventory *RuntimeSourceObligationInventory, profiles []RuntimeProfileContract) error {
	if inventory == nil {
		for _, profile := range profiles {
			for _, requirement := range profile.EvidenceRequirements {
				for _, rule := range requirement.LineageAssertions {
					if rule.ExecutionSourceContractRef != "" {
						return fmt.Errorf("cross-source execution Profile requires confirmed source inventory")
					}
				}
			}
		}
		return nil
	}
	if err := inventory.Validate(); err != nil {
		return err
	}
	sources := make(map[string]RuntimeDefinitionSource, len(inventory.Sources))
	for _, source := range inventory.Sources {
		sources[source.CandidateItemRef] = source
	}
	obligations := make(map[string]RuntimeSourceObligation, len(inventory.Obligations))
	for _, obligation := range inventory.Obligations {
		obligations[obligation.ID] = obligation
	}
	for _, verification := range inventory.Verifications {
		var declaration *RuntimeProfileEvidenceRequirement
		for _, profile := range profiles {
			if profile.ID != verification.ProfileID {
				continue
			}
			for _, value := range profile.EvidenceRequirements {
				if value.ID != verification.EvidenceRequirementID || value.Scope != verification.Scope {
					continue
				}
				if declaration != nil {
					return fmt.Errorf("runtime source verification %q has ambiguous Profile declaration", verification.ID)
				}
				value.TargetSelectorRef, value.TargetSelectorOwnerRef, value.TargetSelectorSubjectRef = ResolveRuntimeProfileTargetSelectorIdentity(profile.ID, value)
				declaration = &value
			}
		}
		if declaration == nil {
			return fmt.Errorf("runtime source verification %q has no exact Profile declaration", verification.ID)
		}
		for _, ref := range obligations[verification.ObligationID].DoDItemRefs {
			source := sources[ref]
			if !RuntimeEvidenceDeclarationMatchesSource(*declaration, source.SourceScope, source.SourceID) {
				return fmt.Errorf("runtime source verification %q does not apply to source %q", verification.ID, ref)
			}
			policy, err := CompileRuntimeSourceEvidencePolicy(source.SourceScope, source.SourceID, profiles, nil)
			if err != nil {
				return fmt.Errorf("runtime source verification %q policy: %w", verification.ID, err)
			}
			if !RuntimeSourceEvidencePolicyRetainsDeclaration(policy, *declaration, source.SourceScope, source.SourceID) {
				return fmt.Errorf("runtime source verification %q loses its declared proof contribution", verification.ID)
			}
		}
	}
	if err := ValidateRuntimeEvidencePurposeMappings(*inventory, profiles); err != nil {
		return err
	}
	return ValidateRuntimeExecutionProofMappings(*inventory, profiles)
}
