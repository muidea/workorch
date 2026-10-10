package skillselectcore

import (
	"strings"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func GoalText(goal *workspacemodel.Goal) string {
	if goal == nil {
		return ""
	}
	return goal.Text
}

func DoDItems(dod *workspacemodel.DoD) []string {
	if dod == nil {
		return nil
	}
	return append([]string(nil), dod.Items...)
}

func AllowedSkillRefs(soul *soulmodel.Soul) []string {
	if soul == nil {
		return nil
	}
	ret := make([]string, 0, len(soul.AllowedSkillRefs))
	for _, value := range soul.AllowedSkillRefs {
		value = strings.TrimSpace(value)
		if value != "" {
			ret = append(ret, value)
		}
	}
	return ret
}

func CandidatesFromSkills(items []workorchmanagementcontract.Skill) []Candidate {
	ret := make([]Candidate, 0, len(items))
	for _, item := range items {
		ret = append(ret, CandidateFromSkill(item))
	}
	return ret
}

func CandidateFromSkill(item workorchmanagementcontract.Skill) Candidate {
	return Candidate{
		ID:            item.ID,
		Name:          item.Name,
		TriggerRules:  append([]string(nil), item.TriggerRules...),
		SourceSkillID: item.SourceSkillID,
		Scope:         item.Scope,
		Source:        item.Source,
	}
}

func SkillsBySelectedCandidates(source []workorchmanagementcontract.Skill, selected []Candidate) []workorchmanagementcontract.Skill {
	if len(selected) == 0 {
		return nil
	}
	byID := make(map[string]workorchmanagementcontract.Skill, len(source))
	for _, item := range source {
		if item.ID != "" {
			byID[item.ID] = item
		}
	}
	ret := make([]workorchmanagementcontract.Skill, 0, len(selected))
	for _, candidate := range selected {
		if item, ok := byID[candidate.ID]; ok {
			ret = append(ret, item)
		}
	}
	return ret
}
