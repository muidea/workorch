package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func resultSummaryArtifactRefs(item workorchresultcontract.ResultSummary) []common.ArtifactID {
	if len(item.ArtifactRefs) == 0 {
		return nil
	}
	ret := make([]common.ArtifactID, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			ret = append(ret, ref)
		}
	}
	return ret
}

func resultSummaryDoDItems(item workorchresultcontract.ResultSummary) []workorchresultcontract.DoDAssessmentItem {
	if len(item.DoDItems) == 0 {
		return nil
	}
	ret := make([]workorchresultcontract.DoDAssessmentItem, 0, len(item.DoDItems))
	for _, assessmentItem := range item.DoDItems {
		ret = append(ret, mapDoDAssessmentItem(assessmentItem))
	}
	return ret
}
