package readmodelprojection

func unlinkedDoDEvidenceRefs(refs []string, evidenceIndex map[string]struct{}) []string {
	missingRefs := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, ok := evidenceIndex[ref]; !ok {
			missingRefs = append(missingRefs, ref)
		}
	}
	return missingRefs
}
