package orchestrator

import (
	"sort"
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func doDAssessmentEvidenceRefs(stepID common.StepID, assessment map[string]any) []string {
	seen := map[string]struct{}{}
	ret := []string{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		if _, exists := seen[ref]; exists {
			return
		}
		seen[ref] = struct{}{}
		ret = append(ret, ref)
	}
	add(string(stepID))
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if ok {
		for _, item := range items {
			for _, ref := range evidenceRefsFromValue(item["evidenceRefs"]) {
				add(ref)
			}
		}
	}
	sort.Strings(ret)
	return ret
}

func resultDoDAssessmentFromMap(assessment map[string]any) resultmodel.DoDAssessment {
	if assessment == nil {
		return resultmodel.DoDAssessment{}
	}
	summary, _ := assessment["summary"].(string)
	satisfied, _ := assessment["satisfied"].(bool)
	ret := resultmodel.DoDAssessment{
		Satisfied: satisfied,
		Summary:   strings.TrimSpace(summary),
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok {
		return ret
	}
	ret.Items = make([]resultmodel.DoDAssessmentItem, 0, len(items))
	for _, item := range items {
		name, _ := item["item"].(string)
		state, _ := item["state"].(string)
		failureReason, _ := item["failureReason"].(string)
		sourceNote, _ := item["sourceNote"].(string)
		ret.Items = append(ret.Items, resultmodel.DoDAssessmentItem{
			Item:          strings.TrimSpace(name),
			State:         strings.TrimSpace(state),
			EvidenceRefs:  evidenceRefsFromValue(item["evidenceRefs"]),
			FailureReason: strings.TrimSpace(failureReason),
			Confidence:    floatFromValue(item["confidence"]),
			SourceNote:    strings.TrimSpace(sourceNote),
		})
	}
	return ret
}
