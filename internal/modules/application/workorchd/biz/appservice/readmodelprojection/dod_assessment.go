package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func mapDoDAssessment(item resultmodel.DoDAssessment) *readmodelcore.DoDAssessment {
	if item.Summary == "" &&
		len(item.Items) == 0 &&
		item.GoalCoverage == nil &&
		item.AcceptanceDimensions == nil &&
		item.DeliverableCoverage == nil &&
		!item.Satisfied {
		return nil
	}
	ret := &readmodelcore.DoDAssessment{
		Satisfied:            item.Satisfied,
		Summary:              item.Summary,
		GoalCoverage:         mapDoDCoverage(item.GoalCoverage),
		AcceptanceDimensions: mapDoDCoverage(item.AcceptanceDimensions),
		DeliverableCoverage:  mapDoDCoverage(item.DeliverableCoverage),
	}
	if len(item.Items) > 0 {
		ret.Items = make([]readmodelcore.DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			ret.Items = append(ret.Items, mapDoDAssessmentItem(assessmentItem))
		}
	}
	return ret
}
