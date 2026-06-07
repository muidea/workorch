package contextregistrycore

import (
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
)

func MapSoul(item *soulmodel.Soul) Soul {
	if item == nil {
		return Soul{}
	}
	return Soul{
		ID:                          string(item.ID),
		Name:                        item.Name,
		Description:                 item.Description,
		BusinessDomains:             CloneStrings(item.BusinessDomains),
		SoulMD:                      item.SoulMD,
		AllowedSkillRefs:            CloneStrings(item.AllowedSkillRefs),
		AllowedMCPServerRefs:        CloneStrings(item.AllowedMCPServerRefs),
		AllowedCapabilityRefs:       CloneStrings(item.AllowedCapabilityRefs),
		AllowedCapabilityCategories: CloneStrings(item.AllowedCapabilityCategories),
		AllowedToolKinds:            CloneStrings(item.AllowedToolKinds),
	}
}

func MapRepositorySkill(item *skillmodel.Skill) Skill {
	if item == nil {
		return Skill{}
	}
	return Skill{
		ID:                     string(item.ID),
		Name:                   item.Name,
		Version:                item.Version,
		Description:            item.Description,
		TriggerRules:           CloneStrings(item.TriggerRules),
		Instruction:            item.Instruction,
		ReferenceRefs:          CloneStrings(item.ReferenceRefs),
		RequiredToolKinds:      CloneStrings(item.RequiredToolKinds),
		RequiredMCPServerRefs:  CloneStrings(item.RequiredMCPServerRefs),
		RequiredCapabilityRefs: CloneStrings(item.RequiredCapabilityRefs),
		Constraints:            CloneStrings(item.Constraints),
		Source:                 SkillSourceRepository,
		Scope:                  SkillScopeRepository,
		ManagedBy:              SkillManagedBy,
		SourceSkillID:          string(item.ID),
	}
}
