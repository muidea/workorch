package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func compressedContextSectionRefsFromBundle(sections []readmodelcore.ContextSection) []string {
	sectionRefs := make([]string, 0, len(sections))
	for _, section := range sections {
		sectionRefs = append(sectionRefs, section.ID)
	}
	return compactUniqueStrings(sectionRefs)
}

func compressedContextSourceEvidenceRefs(progress readmodelcore.TaskProgress, recentEvidence []readmodelcore.ContextEvidence) []string {
	return compactUniqueStrings(append(progress.EvidenceRefs, ContextEvidenceRefs(recentEvidence)...))
}
