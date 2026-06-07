package readmodelprojection

import "strings"

func (r *executionEvidenceRefs) add(target map[string][]string, stepID string, refs ...string) {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return
	}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		target[stepID] = append(target[stepID], ref)
		r.evidenceByStep[stepID] = append(r.evidenceByStep[stepID], ref)
	}
}

func (r *executionEvidenceRefs) compact() {
	for stepID, refs := range r.evidenceByStep {
		r.evidenceByStep[stepID] = compactUniqueStrings(refs)
	}
}
