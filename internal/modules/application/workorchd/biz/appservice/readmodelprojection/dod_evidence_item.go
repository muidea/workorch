package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func dodEvidenceItemFromAssessment(assessmentItem readmodelcore.DoDAssessmentItem, evidenceIndex map[string]struct{}) readmodelcore.DoDEvidenceItem {
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
	return readmodelcore.DoDEvidenceItem{
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
