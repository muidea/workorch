package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func dodEvidenceTraceFromResult(item projectioncontract.Result, evidenceIndex map[string]struct{}) projectioncontract.DoDEvidenceTrace {
	trace := projectioncontract.DoDEvidenceTrace{
		ResultID:  item.ID,
		DoDID:     item.DoDID,
		Satisfied: item.DoDAssessment.Satisfied,
		Summary:   item.DoDAssessment.Summary,
	}
	assessmentItems := item.DoDAssessment.Items
	if len(assessmentItems) == 0 && item.SummaryDetail != nil && len(item.SummaryDetail.DoDItems) > 0 {
		assessmentItems = item.SummaryDetail.DoDItems
	}
	if len(assessmentItems) > 0 {
		trace.Items = make([]projectioncontract.DoDEvidenceItem, 0, len(assessmentItems))
		for _, assessmentItem := range assessmentItems {
			trace.Items = append(trace.Items, dodEvidenceItemFromAssessment(assessmentItem, evidenceIndex))
		}
	}
	return trace
}
