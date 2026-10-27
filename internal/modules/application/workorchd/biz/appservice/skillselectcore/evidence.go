package skillselectcore

import "github.com/muidea/workorch/internal/pkg/skillselection"

func ExplicitEvidence(items []skillselection.Candidate) Evidence {
	ids := make([]string, 0, len(items))
	matches := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
		matches = append(matches, map[string]any{
			"skillId":       item.ID,
			"sourceSkillId": item.SourceSkillID,
			"scope":         item.Scope,
			"source":        item.Source,
		})
	}
	return Evidence{
		Policy:           "explicit_skill_selection",
		Status:           "selected",
		SelectedSkillIDs: ids,
		Matches:          matches,
	}
}
