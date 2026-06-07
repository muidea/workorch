package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func checkResultQualityDoDAssessmentItem(quality *readmodelcore.ResultQualityTrace, assessmentItem readmodelcore.DoDAssessmentItem, evidenceIndex map[string]struct{}, weakValidationRefs map[string]bool) {
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
