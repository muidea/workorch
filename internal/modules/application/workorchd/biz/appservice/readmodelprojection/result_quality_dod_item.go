package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func checkResultQualityDoDAssessmentItem(quality *projectioncontract.ResultQualityTrace, assessmentItem projectioncontract.DoDAssessmentItem, evidenceIndex map[string]struct{}, weakValidationRefs map[string]bool) {
	if strings.TrimSpace(assessmentItem.State) != "satisfied" {
		return
	}
	itemName := strings.TrimSpace(assessmentItem.Item)
	refs := compactUniqueStrings(cloneReadModelStringSlice(assessmentItem.EvidenceRefs))
	if len(refs) == 0 {
		appendResultQualityDoDMissingEvidenceIssue(quality, itemName)
		return
	}
	if missingRefs := unlinkedDoDEvidenceRefs(refs, evidenceIndex); len(missingRefs) > 0 {
		appendResultQualityDoDUnlinkedEvidenceIssue(quality, itemName, missingRefs, refs)
	}
	if allEvidenceRefsWeakValidation(refs, weakValidationRefs) {
		appendResultQualityDoDWeakValidationIssue(quality, itemName, refs)
	}
}
