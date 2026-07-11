package orchestrator

import (
	"sort"
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
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
	for _, ref := range evidenceRefsFromValue(assessment["evidenceRefs"]) {
		add(ref)
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if ok {
		for _, item := range items {
			for _, ref := range evidenceRefsFromValue(item["evidenceRefs"]) {
				add(ref)
			}
		}
	}
	goalCoverage := mapFromValue(assessment["goalCoverage"])
	goalItems, ok := doDAssessmentItemMaps(goalCoverage["items"])
	if ok {
		for _, item := range goalItems {
			for _, ref := range evidenceRefsFromValue(item["evidenceRefs"]) {
				add(ref)
			}
		}
	}
	sort.Strings(ret)
	return ret
}

func completionAssessmentWithCurrentStepValidationEvidenceRef(assessment map[string]any, stepID common.StepID) map[string]any {
	if assessment == nil || stepID == "" {
		return assessment
	}
	stepRef := string(stepID)
	validationRef := stepRef + "/validation/runtime-step"
	ret := cloneStepPayload(assessment)
	replaceRefs := func(item map[string]any) map[string]any {
		if len(item) == 0 {
			return item
		}
		refs := evidenceRefsFromValue(item["evidenceRefs"])
		if len(refs) == 0 {
			return item
		}
		nextRefs := make([]string, 0, len(refs)+1)
		changed := false
		for _, ref := range refs {
			if strings.TrimSpace(ref) == stepRef {
				nextRefs = append(nextRefs, validationRef)
				changed = true
				continue
			}
			nextRefs = append(nextRefs, ref)
		}
		if !changed {
			return item
		}
		next := cloneStepPayload(item)
		next["evidenceRefs"] = compactUniqueStrings(nextRefs)
		return next
	}
	if items, ok := doDAssessmentItemMaps(ret["items"]); ok {
		nextItems := make([]map[string]any, 0, len(items))
		for _, item := range items {
			nextItems = append(nextItems, replaceRefs(item))
		}
		ret["items"] = nextItems
	}
	for _, key := range []string{"goalCoverage", "goalOutcomeCoverage", "acceptanceDimensions", "dimensionCoverage", "deliverableCoverage", "deliverables", "producedArtifacts"} {
		if mapped := mapFromValue(ret[key]); len(mapped) > 0 {
			if items, ok := doDAssessmentItemMaps(mapped["items"]); ok {
				nextMapped := cloneStepPayload(mapped)
				nextItems := make([]map[string]any, 0, len(items))
				for _, item := range items {
					nextItems = append(nextItems, replaceRefs(item))
				}
				nextMapped["items"] = nextItems
				ret[key] = nextMapped
			}
			continue
		}
		if items, ok := doDAssessmentItemMaps(ret[key]); ok {
			nextItems := make([]map[string]any, 0, len(items))
			for _, item := range items {
				nextItems = append(nextItems, replaceRefs(item))
			}
			ret[key] = nextItems
		}
	}
	return ret
}

func rejectedDoDAssessmentWithFallbackEvidenceRefs(stepID common.StepID, assessment map[string]any) map[string]any {
	if assessment == nil {
		return nil
	}
	ret := cloneStepPayload(assessment)
	fallback := doDAssessmentEvidenceRefs(stepID, ret)
	if len(fallback) == 0 && stepID != "" {
		fallback = []string{string(stepID)}
	}
	if len(fallback) == 0 {
		return ret
	}
	if items, ok := doDAssessmentItemMaps(ret["items"]); ok {
		for _, item := range items {
			if strings.TrimSpace(stringFromMap(item, "state")) != "satisfied" &&
				len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 {
				item["evidenceRefs"] = append([]string(nil), fallback...)
			}
		}
		ret["items"] = items
	}
	if goalCoverage := mapFromValue(ret["goalCoverage"]); len(goalCoverage) > 0 {
		if items, ok := doDAssessmentItemMaps(goalCoverage["items"]); ok {
			for _, item := range items {
				if strings.TrimSpace(stringFromMap(item, "state")) != "satisfied" &&
					len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 {
					item["evidenceRefs"] = append([]string(nil), fallback...)
				}
			}
			goalCoverage["items"] = items
			ret["goalCoverage"] = goalCoverage
		}
	}
	return ret
}

func resultDoDAssessmentFromMap(assessment map[string]any) resultmodel.DoDAssessment {
	if assessment == nil {
		return resultmodel.DoDAssessment{}
	}
	summary, _ := assessment["summary"].(string)
	satisfied, _ := assessment["satisfied"].(bool)
	ret := resultmodel.DoDAssessment{
		Satisfied:            satisfied,
		Summary:              strings.TrimSpace(summary),
		GoalCoverage:         resultDoDCoverageFromMap(mapFromValue(assessment["goalCoverage"])),
		AcceptanceDimensions: resultDoDCoverageFromMap(mapFromValue(assessment["acceptanceDimensions"])),
		DeliverableCoverage:  resultDoDCoverageFromMap(mapFromValue(assessment["deliverableCoverage"])),
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

func resultDoDCoverageFromMap(coverage map[string]any) *resultmodel.DoDCoverage {
	if len(coverage) == 0 {
		return nil
	}
	summary, _ := coverage["summary"].(string)
	satisfied, _ := coverage["satisfied"].(bool)
	ret := &resultmodel.DoDCoverage{
		Satisfied: satisfied,
		Summary:   strings.TrimSpace(summary),
	}
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok {
		return ret
	}
	ret.Items = make([]resultmodel.DoDCoverageItem, 0, len(items))
	for _, item := range items {
		ret.Items = append(ret.Items, resultmodel.DoDCoverageItem{
			ID:           strings.TrimSpace(stringFromMap(item, "id")),
			Deliverable:  strings.TrimSpace(stringFromMap(item, "deliverable")),
			Name:         strings.TrimSpace(stringFromMap(item, "name")),
			Path:         strings.TrimSpace(stringFromMap(item, "path")),
			Goal:         strings.TrimSpace(stringFromMap(item, "goal")),
			GoalItem:     strings.TrimSpace(stringFromMap(item, "goalItem")),
			Item:         strings.TrimSpace(stringFromMap(item, "item")),
			Requirement:  strings.TrimSpace(stringFromMap(item, "requirement")),
			State:        strings.TrimSpace(stringFromMap(item, "state")),
			EvidenceRefs: evidenceRefsFromValue(item["evidenceRefs"]),
			ArtifactRefs: resultArtifactRefsFromValue(item["artifactRefs"]),
		})
	}
	return ret
}

func resultArtifactRefsFromValue(value any) []common.ArtifactID {
	refs := stringSliceFromValue(value)
	if len(refs) == 0 {
		return nil
	}
	ret := make([]common.ArtifactID, 0, len(refs))
	seen := map[common.ArtifactID]struct{}{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		id := common.ArtifactID(ref)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, id)
	}
	return ret
}
