package workorchruntimecontract

import "strings"

// EvidenceRevisionAfter reports whether left is strictly newer than right.
// Empty revisions are treated as older than any non-empty revision.
func EvidenceRevisionAfter(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return false
	}
	if right == "" {
		return true
	}
	return left > right
}

func evidenceRevisionAfter(left string, right string) bool {
	return EvidenceRevisionAfter(left, right)
}
