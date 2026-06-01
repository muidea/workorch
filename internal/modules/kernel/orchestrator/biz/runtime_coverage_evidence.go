package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func coverageRequiredEvidenceKindsForSource(run *runModel.Run, source coverageWorkItemSource) []string {
	kinds := []string{"validation"}
	target := map[string]any{
		"kind":                     source.SourceScope,
		"source":                   source.Source,
		"id":                       source.SourceRequirementID,
		"targetText":               source.TargetText,
		"coverageTargetToken":      source.SourceRequirementID,
		"sourceRequirementAliases": source.SourceAliases,
	}
	for _, requirement := range coverageTargetEvidenceKindRequirements(run, []map[string]any{target}) {
		if profileRequirementRequiresStateChangingEvidence(requirement) {
			kinds = append(kinds, "state_change")
		}
		if requirement.RequireDirect || requirement.RequireClaimEvidence {
			kinds = append(kinds, "validation")
		}
	}
	if runExecutionContractRequiresStateChange(run) && coverageSourceCanCarryStateChange(source) {
		kinds = append(kinds, "state_change")
	}
	return compactUniqueStrings(kinds)
}

func coverageSourceCanCarryStateChange(source coverageWorkItemSource) bool {
	if normalizeEvidenceKindRequirementScope(source.SourceScope) != "deliverable" {
		return false
	}
	return normalizedCoverageEvidenceRole(source.EvidenceRole) == "primary_state_change"
}

func normalizedCoverageEvidenceRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	role = strings.ReplaceAll(role, "-", "_")
	role = strings.ReplaceAll(role, " ", "_")
	switch role {
	case "primary", "primary_state_change", "implementation", "direct_state_change":
		return "primary_state_change"
	case "supplemental", "supplemental_evidence", "evidence", "ledger", "report", "snapshot", "validation":
		return "supplemental_evidence"
	default:
		return ""
	}
}

func coverageMissingEvidenceKinds(item map[string]any) []string {
	required := stringSliceFromValue(item["requiredEvidenceKinds"])
	if len(required) == 0 {
		required = []string{"validation"}
	}
	return coverageMissingEvidenceKindsFromDetails(required, mapStringSliceValue(item["acceptedEvidenceByKind"]))
}

func coverageMissingEvidenceKindsFromDetails(requiredKinds []string, acceptedByKind map[string][]string) []string {
	if len(requiredKinds) == 0 {
		requiredKinds = []string{"validation"}
	}
	missing := make([]string, 0, len(requiredKinds))
	for _, kind := range requiredKinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		if len(acceptedByKind[kind]) == 0 {
			missing = append(missing, kind)
		}
	}
	return compactUniqueStrings(missing)
}

func coverageCurrentGap(missingKinds []string) string {
	if len(missingKinds) == 0 {
		return ""
	}
	return "missing accepted evidence binding for required evidence kind: " + strings.Join(missingKinds, ", ")
}

func coverageEvidenceRefsByKind(existing map[string]any, key string, requiredKinds []string) map[string][]string {
	ret := mapStringSliceValue(existing[key])
	if len(ret) > 0 {
		return ret
	}
	if len(requiredKinds) == 1 {
		legacyKey := "acceptedEvidenceRefs"
		if key == "rejectedEvidenceByKind" {
			legacyKey = "rejectedEvidenceRefs"
		}
		if refs := stringSliceFromValue(existing[legacyKey]); len(refs) > 0 {
			ret[requiredKinds[0]] = compactUniqueStrings(refs)
		}
	}
	return ret
}

func coverageFlattenEvidenceRefs(byKind map[string][]string, fallback []string) []string {
	refs := append([]string{}, fallback...)
	kinds := make([]string, 0, len(byKind))
	for kind := range byKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		refs = append(refs, byKind[kind]...)
	}
	return compactUniqueStrings(refs)
}

func coverageWorkItemStepUpdate(step *runModel.Step) coverageWorkItemUpdate {
	if step == nil {
		return coverageWorkItemUpdate{}
	}
	update := coverageWorkItemUpdate{
		WorkItemIDs:  coverageWorkItemIDsFromStep(step),
		TargetTokens: coverageTargetTokensFromStep(step),
		EvidenceRefs: coverageStepEvidenceRefs(step),
		Accepted:     step.State == common.StepStateCompleted,
	}
	if stepKindCanProduceStateChangingEvidence(step.Kind) {
		update.EvidenceKinds = append(update.EvidenceKinds, "state_change")
	}
	if coverageStepCanProduceValidationEvidence(step) {
		update.EvidenceKinds = append(update.EvidenceKinds, "validation")
	}
	update.EvidenceKinds = compactUniqueStrings(update.EvidenceKinds)
	return update
}

func coverageStepCanProduceValidationEvidence(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	switch strings.TrimSpace(step.Kind) {
	case "shell/command", "mcp/tool_call":
		return true
	}
	return len(mapFromValue(step.Input["validationEvidence"])) > 0 ||
		len(mapFromValue(step.Output["validationEvidence"])) > 0 ||
		len(stringSliceFromValue(step.Output["evidenceRefs"])) > 0
}

func coverageStepEvidenceRefs(step *runModel.Step) []string {
	if step == nil || step.ID == "" {
		return nil
	}
	refs := []string{string(step.ID)}
	refs = append(refs, evidenceRefsFromValue(step.Output["evidenceRefs"])...)
	if ref := strings.TrimSpace(stringFromMap(step.Output, "evidenceRef")); ref != "" {
		refs = append(refs, ref)
	}
	if coverageStepCanProduceValidationEvidence(step) {
		refs = append(refs, fmt.Sprintf("%s/validation/runtime-step", step.ID))
	}
	return compactUniqueStrings(refs)
}

func coverageWorkItemIDsFromStep(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	ids := make([]string, 0)
	addFromMap := func(payload map[string]any) {
		if len(payload) == 0 {
			return
		}
		ids = append(ids, stringSliceFromValue(payload["workItemIds"])...)
		ids = append(ids, stringSliceFromValue(payload["coverageWorkItemIds"])...)
		if id := strings.TrimSpace(stringFromMap(payload, "workItemId")); id != "" {
			ids = append(ids, id)
		}
		if id := strings.TrimSpace(stringFromMap(payload, "coverageWorkItemId")); id != "" {
			ids = append(ids, id)
		}
		for _, item := range mapSliceFromValue(payload["coverageTargets"]) {
			ids = append(ids, stringSliceFromValue(item["workItemIds"])...)
			if id := strings.TrimSpace(stringFromMap(item, "workItemId")); id != "" {
				ids = append(ids, id)
			}
		}
	}
	addFromMap(step.Input)
	addFromMap(step.Output)
	for _, patch := range mapSliceFromValue(step.Output["patches"]) {
		addFromMap(patch)
	}
	if decision := mapFromValue(step.Output[stepOutputKeyLLMDecision]); len(decision) > 0 {
		addFromMap(decision)
		for _, patch := range mapSliceFromValue(decision["patchRequests"]) {
			addFromMap(patch)
		}
	}
	return compactUniqueStrings(ids)
}

func coverageWorkItemAssessmentUpdates(step *runModel.Step, assessment map[string]any) []coverageWorkItemUpdate {
	if step == nil || len(assessment) == 0 {
		return nil
	}
	updates := make([]coverageWorkItemUpdate, 0)
	addItems := func(items []map[string]any) {
		for _, item := range items {
			tokens := coverageTokensFromAssessmentItem(item)
			if len(tokens) == 0 {
				continue
			}
			refs := evidenceRefsFromValue(item["evidenceRefs"])
			if len(refs) == 0 {
				refs = []string{string(step.ID)}
			}
			state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
			accepted := state == "satisfied"
			update := coverageWorkItemUpdate{
				TargetTokens:   tokens,
				EvidenceKinds:  []string{"validation"},
				EvidenceRefs:   refs,
				Accepted:       accepted,
				CloseSatisfied: accepted,
				Reason:         firstNonEmptyString(stringFromMap(item, "reason"), stringFromMap(item, "failureReason"), stringFromMap(item, "summary")),
			}
			updates = append(updates, update)
		}
	}
	if items, ok := doDAssessmentItemMaps(assessment["items"]); ok {
		addItems(items)
	}
	for _, key := range []string{"goalCoverage", "deliverableCoverage", "acceptanceDimensions"} {
		coverage := mapFromValue(assessment[key])
		if items, ok := doDAssessmentItemMaps(coverage["items"]); ok {
			addItems(items)
		}
	}
	return updates
}

func coverageTokensFromAssessmentItem(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	tokens := make([]string, 0)
	for _, key := range []string{
		"workItemId",
		"coverageWorkItemId",
		"coverageTargetToken",
		"goalRequirementId",
		"id",
		"item",
		"goal",
		"goalItem",
		"requirement",
		"deliverable",
		"name",
		"path",
		"targetText",
	} {
		tokens = append(tokens, stringFromMap(item, key))
	}
	tokens = append(tokens, stringSliceFromValue(item["workItemIds"])...)
	tokens = append(tokens, stringSliceFromValue(item["coverageWorkItemIds"])...)
	return compactUniqueStrings(tokens)
}

func coverageWorkItemMatchesUpdate(item map[string]any, update coverageWorkItemUpdate) bool {
	if len(item) == 0 {
		return false
	}
	id := strings.TrimSpace(stringFromMap(item, "id"))
	for _, candidate := range update.WorkItemIDs {
		if strings.TrimSpace(candidate) == id {
			return true
		}
	}
	candidates := []string{
		stringFromMap(item, "normalizedSourceKey"),
		stringFromMap(item, "sourceRequirementId"),
		stringFromMap(item, "targetText"),
		stringFromMap(item, "sourcePath"),
	}
	candidates = append(candidates, stringSliceFromValue(item["sourceRequirementAliases"])...)
	for _, token := range update.TargetTokens {
		for _, candidate := range candidates {
			if coverageRepairTokenMatches(token, candidate) {
				return true
			}
		}
	}
	return false
}

func coverageApplyWorkItemEvidence(item map[string]any, kinds []string, refs []string, accepted bool, closeSatisfied bool, stepID string, reason string) bool {
	if len(item) == 0 || len(kinds) == 0 || len(refs) == 0 {
		return false
	}
	required := stringSet(stringSliceFromValue(item["requiredEvidenceKinds"]))
	changed := false
	key := "rejectedEvidenceByKind"
	flatKey := "rejectedEvidenceRefs"
	if accepted {
		key = "acceptedEvidenceByKind"
		flatKey = "acceptedEvidenceRefs"
	}
	byKind := mapStringSliceValue(item[key])
	for _, kind := range kinds {
		kind = strings.TrimSpace(kind)
		if kind == "" || !required[kind] {
			continue
		}
		before := stableCoverageJSON(byKind[kind])
		byKind[kind] = compactUniqueStrings(append(byKind[kind], refs...))
		if before != stableCoverageJSON(byKind[kind]) {
			changed = true
		}
	}
	item[key] = byKind
	item[flatKey] = coverageFlattenEvidenceRefs(byKind, stringSliceFromValue(item[flatKey]))
	if stepID != "" {
		item["updatedByStepId"] = stepID
	}
	item["updatedAt"] = runtimeEvidenceTimestamp()
	if strings.TrimSpace(reason) != "" {
		item["reason"] = reason
	}
	missing := coverageMissingEvidenceKinds(item)
	item["currentGap"] = coverageCurrentGap(missing)
	if accepted {
		if len(missing) == 0 {
			if closeSatisfied {
				if strings.TrimSpace(stringFromMap(item, "status")) != "satisfied" {
					changed = true
				}
				if strings.TrimSpace(stringFromMap(item, "currentGap")) != "" {
					changed = true
				}
				item["status"] = "satisfied"
				item["currentGap"] = ""
			} else if strings.TrimSpace(stringFromMap(item, "status")) != "satisfied" {
				if strings.TrimSpace(stringFromMap(item, "status")) != "evidence_ready" {
					changed = true
					item["status"] = "evidence_ready"
				}
			}
		} else if strings.TrimSpace(stringFromMap(item, "status")) == "" || strings.TrimSpace(stringFromMap(item, "status")) == "open" {
			changed = true
			item["status"] = "in_progress"
		}
	} else {
		if strings.TrimSpace(stringFromMap(item, "status")) != "failed" {
			changed = true
		}
		item["status"] = "failed"
		if strings.TrimSpace(reason) != "" {
			if strings.TrimSpace(stringFromMap(item, "currentGap")) != reason {
				changed = true
			}
			item["currentGap"] = reason
		}
	}
	if !changed {
		return false
	}
	return true
}
