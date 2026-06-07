package readmodelprojection

import "strings"

type runEvidenceRefs map[string]struct{}

func (refs runEvidenceRefs) add(ref string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return
	}
	refs[ref] = struct{}{}
}
