package activationcore

import "strings"

func DefinitionRevisionIDFromExecutionContext(input map[string]any) string {
	revision, _ := input["definitionRevision"].(map[string]any)
	if len(revision) == 0 {
		return ""
	}
	if id := strings.TrimSpace(stringFromMap(revision, "id")); id != "" {
		return id
	}
	goalID := strings.TrimSpace(stringFromMap(revision, "effectiveGoalId"))
	doDID := strings.TrimSpace(stringFromMap(revision, "effectiveDoDId"))
	if goalID == "" && doDID == "" {
		return ""
	}
	return "goal:" + goalID + "|dod:" + doDID
}
