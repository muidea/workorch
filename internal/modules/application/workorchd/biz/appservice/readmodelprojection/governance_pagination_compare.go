package readmodelprojection

import "strings"

func governancePageCreatedIDBefore(leftCreated string, rightCreated string, leftID string, rightID string) bool {
	leftCreated = strings.TrimSpace(leftCreated)
	rightCreated = strings.TrimSpace(rightCreated)
	if leftCreated != "" || rightCreated != "" {
		if leftCreated != rightCreated {
			return leftCreated > rightCreated
		}
	}
	return leftID > rightID
}
