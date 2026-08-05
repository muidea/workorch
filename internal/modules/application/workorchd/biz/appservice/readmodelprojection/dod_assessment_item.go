package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchresultcontract"

func mapDoDAssessmentItem(item workorchresultcontract.DoDAssessmentItem) workorchresultcontract.DoDAssessmentItem {
	return workorchresultcontract.DoDAssessmentItem{
		Item:          item.Item,
		State:         item.State,
		EvidenceRefs:  cloneReadModelStringSlice(item.EvidenceRefs),
		FailureReason: item.FailureReason,
		Confidence:    item.Confidence,
		SourceNote:    item.SourceNote,
	}
}
