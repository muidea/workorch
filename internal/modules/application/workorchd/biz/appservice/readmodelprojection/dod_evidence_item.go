package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func dodEvidenceItemFromAssessment(assessmentItem workorchresultcontract.DoDAssessmentItem, evidenceIndex map[string]struct{}) projectioncontract.DoDEvidenceItem {
	refs := compactUniqueStrings(cloneReadModelStringSlice(assessmentItem.EvidenceRefs))
	missing := make([]string, 0)
	for _, ref := range refs {
		if _, ok := evidenceIndex[ref]; !ok {
			missing = append(missing, ref)
		}
	}
	status := "missing"
	if len(refs) > 0 && len(missing) == 0 {
		status = "linked"
	} else if len(refs) > 0 && len(missing) < len(refs) {
		status = "partial"
	}
	return projectioncontract.DoDEvidenceItem{
		Item:                assessmentItem.Item,
		State:               assessmentItem.State,
		FailureReason:       assessmentItem.FailureReason,
		Confidence:          assessmentItem.Confidence,
		SourceNote:          assessmentItem.SourceNote,
		EvidenceRefs:        refs,
		EvidenceStatus:      status,
		MissingEvidenceRefs: missing,
	}
}
