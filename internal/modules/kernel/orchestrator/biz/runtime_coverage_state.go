package orchestrator

import (
	"strings"
)

const (
	coverageWorkItemActionNone           = "none"
	coverageWorkItemActionNeedEvidence   = "need_evidence"
	coverageWorkItemActionNeedAssessment = "need_assessment"
	coverageWorkItemActionNeedDecision   = "need_decision"
)

type coverageWorkItemDerivedState struct {
	Status                string
	MissingEvidenceKinds  []string
	RequiredEvidenceKinds []string
	AcceptedEvidenceRefs  []string
	CurrentGap            string
	NextAction            string
}

func deriveCoverageWorkItemState(item map[string]any) coverageWorkItemDerivedState {
	ret := coverageWorkItemDerivedState{NextAction: coverageWorkItemActionNone}
	if len(item) == 0 {
		return ret
	}
	ret.Status = strings.ToLower(strings.TrimSpace(stringFromMap(item, "status")))
	ret.RequiredEvidenceKinds = coverageWorkItemEffectiveRequiredEvidenceKinds(item)
	ret.AcceptedEvidenceRefs = stringSliceFromValue(item["acceptedEvidenceRefs"])
	ret.CurrentGap = strings.TrimSpace(stringFromMap(item, "currentGap"))

	switch ret.Status {
	case "skipped":
		return ret
	case "blocked":
		ret.NextAction = coverageWorkItemActionNeedDecision
		return ret
	}
	ret.MissingEvidenceKinds = coverageWorkItemDerivedMissingEvidenceKinds(item)
	if len(ret.MissingEvidenceKinds) > 0 {
		ret.NextAction = coverageWorkItemActionNeedEvidence
		return ret
	}
	if coverageWorkItemHasRequiredEvidence(item) && !coverageWorkItemRequiredEvidenceKindsSatisfied(item) {
		ret.NextAction = coverageWorkItemActionNeedEvidence
		return ret
	}
	switch ret.Status {
	case "satisfied", "closed", "completed":
		return ret
	}
	if ret.Status == "evidence_ready" {
		ret.NextAction = coverageWorkItemActionNeedAssessment
		return ret
	}
	if coverageWorkItemHasRequiredEvidence(item) && coverageWorkItemRequiredEvidenceComplete(item) {
		ret.NextAction = coverageWorkItemActionNeedAssessment
		return ret
	}
	return ret
}

func (s coverageWorkItemDerivedState) HasActionableEvidenceGap() bool {
	return s.NextAction == coverageWorkItemActionNeedEvidence
}

func (s coverageWorkItemDerivedState) NeedsAssessment() bool {
	return s.NextAction == coverageWorkItemActionNeedAssessment
}

func coverageWorkItemHasRequiredEvidence(item map[string]any) bool {
	return len(coverageWorkItemEffectiveRequiredEvidenceKinds(item)) > 0
}

func coverageWorkItemRequiredEvidenceComplete(item map[string]any) bool {
	if len(coverageWorkItemDerivedMissingEvidenceKinds(item)) != 0 {
		return false
	}
	if !coverageWorkItemHasRequiredEvidence(item) {
		return true
	}
	return coverageWorkItemRequiredEvidenceKindsSatisfied(item)
}

func coverageWorkItemDerivedMissingEvidenceKinds(item map[string]any) []string {
	if item == nil {
		return nil
	}
	if _, ok := item["missingEvidenceKinds"]; ok {
		missing := coverageWorkItemNormalizeExplicitMissingEvidenceKinds(item, stringSliceFromValue(item["missingEvidenceKinds"]))
		if len(missing) == 0 && coverageWorkItemHasRequiredEvidence(item) && !coverageWorkItemRequiredEvidenceKindsSatisfied(item) {
			return coverageWorkItemMissingEvidenceKindsFromAcceptedEvidence(item)
		}
		return missing
	}
	return coverageWorkItemMissingEvidenceKindsFromAcceptedEvidence(item)
}

func coverageWorkItemMissingEvidenceKindsFromAcceptedEvidence(item map[string]any) []string {
	required := coverageWorkItemEffectiveRequiredEvidenceKinds(item)
	requiredCounts := mapIntValue(item["requiredEvidenceCounts"])
	if coverageWorkItemFlatAcceptedRefsSatisfyRequiredKinds(item, required, requiredCounts) {
		return nil
	}
	missing := make([]string, 0, len(required))
	for _, kind := range required {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		requiredCount := requiredCounts[kind]
		if requiredCount <= 0 {
			requiredCount = 1
		}
		if coverageWorkItemAcceptedEvidenceCountForKind(item, kind, required) < requiredCount {
			missing = append(missing, kind)
		}
	}
	return compactUniqueStrings(missing)
}

func coverageWorkItemRequiredEvidenceKindsSatisfied(item map[string]any) bool {
	return len(coverageWorkItemMissingEvidenceKindsFromAcceptedEvidence(item)) == 0
}

func coverageWorkItemAcceptedEvidenceCountForKind(item map[string]any, kind string, requiredKinds []string) int {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return 0
	}
	acceptedByKind := mapStringSliceValue(item["acceptedEvidenceByKind"])
	if refs := acceptedByKind[kind]; len(refs) > 0 {
		return coverageEvidenceCountForKind(kind, refs)
	}
	requiredKinds = compactUniqueStrings(requiredKinds)
	if len(requiredKinds) == 1 && strings.TrimSpace(requiredKinds[0]) == kind {
		return coverageEvidenceCountForKind(kind, stringSliceFromValue(item["acceptedEvidenceRefs"]))
	}
	return 0
}

func coverageWorkItemFlatAcceptedRefsSatisfyRequiredKinds(item map[string]any, requiredKinds []string, requiredCounts map[string]int) bool {
	status := strings.ToLower(strings.TrimSpace(stringFromMap(item, "status")))
	switch status {
	case "evidence_ready", "satisfied", "closed", "completed":
	default:
		return false
	}
	if len(mapStringSliceValue(item["acceptedEvidenceByKind"])) > 0 {
		return false
	}
	refs := compactUniqueStrings(stringSliceFromValue(item["acceptedEvidenceRefs"]))
	if len(refs) == 0 {
		return false
	}
	requiredKinds = compactUniqueStrings(requiredKinds)
	requiredTotal := 0
	for _, kind := range requiredKinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		requiredCount := requiredCounts[kind]
		if requiredCount <= 0 {
			requiredCount = 1
		}
		requiredTotal += requiredCount
	}
	return requiredTotal > 0 && len(refs) >= requiredTotal
}

func coverageWorkItemNormalizeExplicitMissingEvidenceKinds(item map[string]any, missing []string) []string {
	missing = compactUniqueStrings(missing)
	if len(missing) == 0 || len(item) == 0 {
		return missing
	}
	missingSet := stringSet(missing)
	requiredCounts := mapIntValue(item["requiredEvidenceCounts"])
	requiredKinds := coverageWorkItemEffectiveRequiredEvidenceKinds(item)
	if coverageWorkItemFlatAcceptedRefsSatisfyRequiredKinds(item, requiredKinds, requiredCounts) {
		return nil
	}
	for _, kind := range requiredKinds {
		if !missingSet[kind] {
			continue
		}
		requiredCount := requiredCounts[kind]
		if requiredCount <= 0 {
			requiredCount = 1
		}
		if coverageWorkItemAcceptedEvidenceCountForKind(item, kind, requiredKinds) >= requiredCount {
			delete(missingSet, kind)
		}
	}
	ret := make([]string, 0, len(missing))
	for _, kind := range missing {
		if missingSet[kind] {
			ret = append(ret, kind)
		}
	}
	return ret
}

func coverageWorkItemHasAcceptedEvidence(item map[string]any) bool {
	if len(stringSliceFromValue(item["acceptedEvidenceRefs"])) > 0 {
		return true
	}
	for _, refs := range mapStringSliceValue(item["acceptedEvidenceByKind"]) {
		if len(compactUniqueStrings(refs)) > 0 {
			return true
		}
	}
	return false
}

func coverageWorkItemMissingEvidenceKindSet(item map[string]any) map[string]bool {
	return stringSet(deriveCoverageWorkItemState(item).MissingEvidenceKinds)
}

func coverageWorkItemEffectiveRequiredEvidenceKinds(item map[string]any) []string {
	required := stringSliceFromValue(item["requiredEvidenceKinds"])
	if len(required) == 0 {
		required = []string{"validation"}
	}
	return compactUniqueStrings(required)
}
