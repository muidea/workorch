package activationcore

import "strings"

func DefinitionRevisionID(goalID string, doDID string) string {
	goalID = strings.TrimSpace(goalID)
	doDID = strings.TrimSpace(doDID)
	if goalID == "" && doDID == "" {
		return ""
	}
	return "goal:" + goalID + "|dod:" + doDID
}
