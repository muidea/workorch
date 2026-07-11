package contextregistryports

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func soulCommandFromView(cmd contextregistrycore.Soul) soulevents.CreateSoulCommand {
	return soulevents.CreateSoulCommand{
		SoulID:                      common.SoulID(strings.TrimSpace(cmd.ID)),
		Name:                        cmd.Name,
		Description:                 cmd.Description,
		BusinessDomains:             contextregistrycore.CloneStrings(cmd.BusinessDomains),
		SoulMD:                      cmd.SoulMD,
		AllowedSkillRefs:            contextregistrycore.CloneStrings(cmd.AllowedSkillRefs),
		AllowedMCPServerRefs:        contextregistrycore.CloneStrings(cmd.AllowedMCPServerRefs),
		AllowedCapabilityRefs:       contextregistrycore.CloneStrings(cmd.AllowedCapabilityRefs),
		AllowedCapabilityCategories: contextregistrycore.CloneStrings(cmd.AllowedCapabilityCategories),
		AllowedToolKinds:            contextregistrycore.CloneStrings(cmd.AllowedToolKinds),
	}
}

func skillCommandFromView(cmd contextregistrycore.Skill) skillevents.CreateSkillCommand {
	return skillevents.CreateSkillCommand{
		SkillID:                common.SkillID(strings.TrimSpace(cmd.ID)),
		Name:                   cmd.Name,
		Version:                cmd.Version,
		Description:            cmd.Description,
		TriggerRules:           contextregistrycore.CloneStrings(cmd.TriggerRules),
		Instruction:            cmd.Instruction,
		ReferenceRefs:          contextregistrycore.CloneStrings(cmd.ReferenceRefs),
		RequiredToolKinds:      contextregistrycore.CloneStrings(cmd.RequiredToolKinds),
		RequiredMCPServerRefs:  contextregistrycore.CloneStrings(cmd.RequiredMCPServerRefs),
		RequiredCapabilityRefs: contextregistrycore.CloneStrings(cmd.RequiredCapabilityRefs),
		Constraints:            contextregistrycore.CloneStrings(cmd.Constraints),
	}
}
