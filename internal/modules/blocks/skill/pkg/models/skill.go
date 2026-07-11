package skill

import "github.com/muidea/workorch/internal/pkg/common"

type Skill struct {
	ID                     common.SkillID
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
