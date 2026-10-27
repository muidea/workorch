package skillselection

import "strings"

func SearchText(goalText string, dodItems []string) string {
	parts := []string{}
	if strings.TrimSpace(goalText) != "" {
		parts = append(parts, goalText)
	}
	parts = append(parts, dodItems...)
	return NormalizeMatchText(strings.Join(parts, " "))
}

func ScoreCandidate(skill Candidate, searchText string) (int, []string) {
	if searchText == "" {
		return 0, nil
	}
	score := 0
	reasons := []string{}
	for _, rule := range skill.TriggerRules {
		ruleText := NormalizeMatchText(rule)
		if ruleText == "" {
			continue
		}
		if strings.Contains(searchText, ruleText) {
			score += 10
			reasons = append(reasons, "trigger:"+rule)
		}
	}
	for _, value := range []string{skill.Name, skill.SourceSkillID} {
		valueText := NormalizeMatchText(value)
		if valueText == "" {
			continue
		}
		if strings.Contains(searchText, valueText) {
			score += 4
			reasons = append(reasons, "name:"+value)
		}
	}
	return score, reasons
}

func NormalizeMatchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("-", " ", "_", " ", "/", " ", ".", " ", ",", " ", ":", " ", ";", " ", "\n", " ", "\t", " ")
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}
