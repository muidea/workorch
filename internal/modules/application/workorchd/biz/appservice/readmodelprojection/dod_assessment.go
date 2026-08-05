package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchresultcontract"

func mapDoDAssessment(item workorchresultcontract.DoDAssessment) *workorchresultcontract.DoDAssessment {
	if item.Summary == "" &&
		len(item.Items) == 0 &&
		item.GoalCoverage == nil &&
		item.AcceptanceDimensions == nil &&
		item.DeliverableCoverage == nil &&
		!item.Satisfied {
		return nil
	}
	ret := &workorchresultcontract.DoDAssessment{
		Satisfied:            item.Satisfied,
		Summary:              item.Summary,
		GoalCoverage:         mapDoDCoverage(item.GoalCoverage),
		AcceptanceDimensions: mapDoDCoverage(item.AcceptanceDimensions),
		DeliverableCoverage:  mapDoDCoverage(item.DeliverableCoverage),
	}
	if len(item.Items) > 0 {
		ret.Items = make([]workorchresultcontract.DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			ret.Items = append(ret.Items, mapDoDAssessmentItem(assessmentItem))
		}
	}
	return ret
}
