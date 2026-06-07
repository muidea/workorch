package readmodelprojection

import "strings"

func graphRootStepIDFromStepID(stepID string) string {
	idx := strings.LastIndex(stepID, "/node/")
	if idx <= 0 {
		return ""
	}
	return stepID[:idx]
}
