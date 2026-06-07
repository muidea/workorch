package contracts

import "github.com/muidea/workorch/internal/pkg/common"

type CreateSkillCommand struct {
	SkillID                common.SkillID
	Name                   string
	Version                string
	Description            string
	TriggerRules           []string
	Instruction            string
	ReferenceRefs          []string
	RequiredToolKinds      []string
	RequiredMCPServerRefs  []string
	RequiredCapabilityRefs []string
	Constraints            []string
}

type UpdateSkillCommand = CreateSkillCommand
