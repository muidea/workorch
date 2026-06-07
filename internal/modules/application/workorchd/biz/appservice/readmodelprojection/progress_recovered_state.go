package readmodelprojection

import "strings"

func RecoveredProgressItemState(state string, recoveredHistoricalFailures bool) string {
	if recoveredHistoricalFailures && strings.TrimSpace(state) == "failed" {
		return "recovered"
	}
	return state
}
