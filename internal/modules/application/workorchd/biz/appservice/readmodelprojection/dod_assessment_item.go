package readmodelprojection

import (
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapDoDAssessmentItem(item resultmodel.DoDAssessmentItem) projectioncontract.DoDAssessmentItem {
	return projectioncontract.DoDAssessmentItem{
		Item:          item.Item,
		State:         item.State,
		EvidenceRefs:  cloneReadModelStringSlice(item.EvidenceRefs),
		FailureReason: item.FailureReason,
		Confidence:    item.Confidence,
		SourceNote:    item.SourceNote,
	}
}
