package contextregistrycore

import (
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func MapSoul(item *soulmodel.Soul) workorchmanagementcontract.Soul {
	if item == nil {
		return workorchmanagementcontract.Soul{}
	}
	return workorchmanagementcontract.Soul{
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

func MapRepositorySkill(item *skillmodel.Skill) workorchmanagementcontract.Skill {
	if item == nil {
		return workorchmanagementcontract.Skill{}
	}
	return workorchmanagementcontract.Skill{
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
