package readmodelprojection

import (
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func resultSummaryArtifactRefs(item resultmodel.ResultSummary) []string {
	if len(item.ArtifactRefs) == 0 {
		return nil
	}
	ret := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			ret = append(ret, string(ref))
		}
	}
	return ret
}

func resultSummaryDoDItems(item resultmodel.ResultSummary) []projectioncontract.DoDAssessmentItem {
	if len(item.DoDItems) == 0 {
		return nil
	}
	ret := make([]projectioncontract.DoDAssessmentItem, 0, len(item.DoDItems))
	for _, assessmentItem := range item.DoDItems {
		ret = append(ret, mapDoDAssessmentItem(assessmentItem))
	}
	return ret
}
