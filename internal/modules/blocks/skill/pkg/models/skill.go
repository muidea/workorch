package skill

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
	"slices"
)

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
	Provenance             workorchskillcontract.Provenance
}

func (s *Skill) Content() workorchskillcontract.Content {
	if s == nil {
		return workorchskillcontract.Content{}
	}
	return workorchskillcontract.Content{
		ID: string(s.ID), Name: s.Name, Version: s.Version, Description: s.Description,
		TriggerRules: append([]string(nil), s.TriggerRules...), Instruction: s.Instruction,
		ReferenceRefs: append([]string(nil), s.ReferenceRefs...), RequiredToolKinds: append([]string(nil), s.RequiredToolKinds...),
		RequiredMCPServerRefs: append([]string(nil), s.RequiredMCPServerRefs...), RequiredCapabilityRefs: append([]string(nil), s.RequiredCapabilityRefs...),
		Constraints: append([]string(nil), s.Constraints...),
	}
}

func CloneSkill(item *Skill) *Skill {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.TriggerRules = slices.Clone(item.TriggerRules)
	cloned.ReferenceRefs = slices.Clone(item.ReferenceRefs)
	cloned.RequiredToolKinds = slices.Clone(item.RequiredToolKinds)
	cloned.RequiredMCPServerRefs = slices.Clone(item.RequiredMCPServerRefs)
	cloned.RequiredCapabilityRefs = slices.Clone(item.RequiredCapabilityRefs)
	cloned.Constraints = slices.Clone(item.Constraints)
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
