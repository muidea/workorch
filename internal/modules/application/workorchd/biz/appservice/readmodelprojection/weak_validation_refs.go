package readmodelprojection

import "strings"

func allEvidenceRefsWeakValidation(refs []string, weakRefIndex map[string]bool) bool {
	if len(refs) == 0 || len(weakRefIndex) == 0 {
		return false
	}
	seen := false
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		seen = true
		if !weakRefIndex[ref] {
			return false
		}
	}
	return seen
}
