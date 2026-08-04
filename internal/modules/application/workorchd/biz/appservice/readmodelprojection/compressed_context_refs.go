package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func compressedContextSectionRefsFromBundle(sections []projectioncontract.ContextSection) []string {
	sectionRefs := make([]string, 0, len(sections))
	for _, section := range sections {
		sectionRefs = append(sectionRefs, section.ID)
	}
	return compactUniqueStrings(sectionRefs)
}

func compressedContextSourceEvidenceRefs(progress projectioncontract.TaskProgress, recentEvidence []projectioncontract.ContextEvidence) []string {
	return compactUniqueStrings(append(progress.EvidenceRefs, ContextEvidenceRefs(recentEvidence)...))
}
