package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
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

func resultSummaryDoDItems(item resultmodel.ResultSummary) []readmodelcore.DoDAssessmentItem {
	if len(item.DoDItems) == 0 {
		return nil
	}
	ret := make([]readmodelcore.DoDAssessmentItem, 0, len(item.DoDItems))
	for _, assessmentItem := range item.DoDItems {
		ret = append(ret, mapDoDAssessmentItem(assessmentItem))
	}
	return ret
}
