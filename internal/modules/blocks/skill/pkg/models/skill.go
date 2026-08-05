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

func CloneSkill(item *Skill) *Skill {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.TriggerRules = append([]string(nil), item.TriggerRules...)
	cloned.ReferenceRefs = append([]string(nil), item.ReferenceRefs...)
	cloned.RequiredToolKinds = append([]string(nil), item.RequiredToolKinds...)
	cloned.RequiredMCPServerRefs = append([]string(nil), item.RequiredMCPServerRefs...)
	cloned.RequiredCapabilityRefs = append([]string(nil), item.RequiredCapabilityRefs...)
	cloned.Constraints = append([]string(nil), item.Constraints...)
	return &cloned
}

func CloneSkills(items []*Skill) []*Skill {
	if items == nil {
		return nil
	}
	ret := make([]*Skill, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneSkill(item))
	}
	return ret
}
