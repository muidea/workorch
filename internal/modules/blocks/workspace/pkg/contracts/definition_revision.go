package contracts

import "strings"

// ExecutionDefinitionRevision is the immutable identity of the exact Goal
// and DoD content selected by the Workspace owner. Resource IDs alone are not
// sufficient because Goal and DoD owners update their content revisions while
// retaining stable resource IDs.
func ExecutionDefinitionRevision(goalID, goalRevision, doDID, doDRevision string) string {
	goalID = strings.TrimSpace(goalID)
	goalRevision = strings.TrimSpace(goalRevision)
	doDID = strings.TrimSpace(doDID)
	doDRevision = strings.TrimSpace(doDRevision)
	if goalID == "" || goalRevision == "" || doDID == "" || doDRevision == "" {
		return ""
	}
	return "goal:" + goalID + "@" + goalRevision + "|dod:" + doDID + "@" + doDRevision
}
