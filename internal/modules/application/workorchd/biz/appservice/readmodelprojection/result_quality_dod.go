package readmodelprojection

import (
	"fmt"
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func checkResultQualityDoDAssessment(quality *projectioncontract.ResultQualityTrace, item projectioncontract.Result, requiredDoDLeafItems []string, evidenceIndex map[string]struct{}, weakValidationRefs map[string]bool, evidenceRefs []string) {
	if item.DoDAssessment == nil || len(item.DoDAssessment.Items) == 0 {
		appendResultQualityIssue(quality, "missing_dod_item_assessment", "warning", "result is missing DoD item-level assessment", "attach DoD item states and evidence refs", evidenceRefs...)
		return
	}
	if missingLeaf := missingRequiredDoDLeafItems(requiredDoDLeafItems, item.DoDAssessment.Items); len(missingLeaf) > 0 {
		appendResultQualityIssue(
			quality,
			"missing_dod_leaf_coverage",
			"error",
			fmt.Sprintf("result dodAssessment does not cover required DoD leaf items: %s", strings.Join(missingLeaf, "; ")),
			"include each missing DoD leaf item in dodAssessment.items with concrete evidenceRefs",
			evidenceRefs...,
		)
	}
	for _, assessmentItem := range item.DoDAssessment.Items {
		checkResultQualityDoDAssessmentItem(quality, assessmentItem, evidenceIndex, weakValidationRefs)
	}
}
