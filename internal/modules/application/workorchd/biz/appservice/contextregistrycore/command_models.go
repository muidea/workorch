package contextregistrycore

import (
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func SoulModelFromCommand(cmd Soul) *soulmodel.Soul {
	return &soulmodel.Soul{
		ID:                          common.SoulID(cmd.ID),
		Name:                        cmd.Name,
		Description:                 cmd.Description,
		BusinessDomains:             CloneStrings(cmd.BusinessDomains),
		SoulMD:                      cmd.SoulMD,
		AllowedSkillRefs:            CloneStrings(cmd.AllowedSkillRefs),
		AllowedMCPServerRefs:        CloneStrings(cmd.AllowedMCPServerRefs),
		AllowedCapabilityRefs:       CloneStrings(cmd.AllowedCapabilityRefs),
		AllowedCapabilityCategories: CloneStrings(cmd.AllowedCapabilityCategories),
		AllowedToolKinds:            CloneStrings(cmd.AllowedToolKinds),
	}
}

func SkillModelFromCommand(cmd Skill) *skillmodel.Skill {
	return &skillmodel.Skill{
		ID:                     common.SkillID(cmd.ID),
		Name:                   cmd.Name,
		Version:                cmd.Version,
		Description:            cmd.Description,
		TriggerRules:           CloneStrings(cmd.TriggerRules),
		Instruction:            cmd.Instruction,
		ReferenceRefs:          CloneStrings(cmd.ReferenceRefs),
		RequiredToolKinds:      CloneStrings(cmd.RequiredToolKinds),
		RequiredMCPServerRefs:  CloneStrings(cmd.RequiredMCPServerRefs),
		RequiredCapabilityRefs: CloneStrings(cmd.RequiredCapabilityRefs),
		Constraints:            CloneStrings(cmd.Constraints),
	}
}
