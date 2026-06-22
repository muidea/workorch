package readmodelprojection

import "strings"

const governanceCursorSeparator = "\x00"

func governanceScopedCursor(scope string, id string) string {
	scope = strings.TrimSpace(scope)
	id = strings.TrimSpace(id)
	if scope == "" {
		return id
	}
	return scope + governanceCursorSeparator + id
}

func governanceCursorMatches(cursor string, scope string, id string) bool {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return false
	}
	return cursor == governanceScopedCursor(scope, id)
}
