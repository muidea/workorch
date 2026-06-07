package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func dodEvidenceTraceFromResult(item readmodelcore.Result, evidenceIndex map[string]struct{}) readmodelcore.DoDEvidenceTrace {
	trace := readmodelcore.DoDEvidenceTrace{
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
		trace.Items = make([]readmodelcore.DoDEvidenceItem, 0, len(assessmentItems))
		for _, assessmentItem := range assessmentItems {
			trace.Items = append(trace.Items, dodEvidenceItemFromAssessment(assessmentItem, evidenceIndex))
		}
	}
	return trace
}
