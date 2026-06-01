package orchestrator

import (
	"sort"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func coverageWorkItemContextFromItems(revision string, items []map[string]any, excludedSources []map[string]any, historicalItems []map[string]any) map[string]any {
	if len(items) == 0 && len(excludedSources) == 0 {
		return nil
	}
	summary := map[string]any{"total": len(items)}
	var currentFocus map[string]any
	currentFocusItems := make([]map[string]any, 0)
	unresolved := make([]map[string]any, 0)
	ledgerSummary := make([]map[string]any, 0)
	focusIDs := map[string]bool{}
	openItems := make([]map[string]any, 0)
	for _, item := range items {
		status := strings.TrimSpace(stringFromMap(item, "status"))
		if status == "" {
			status = "open"
		}
		summary[status] = intFromAny(summary[status]) + 1
		if !coverageWorkItemClosedForFocus(status) {
			openItems = append(openItems, item)
			unresolved = append(unresolved, coverageWorkItemSummary(item))
		}
	}
	sort.SliceStable(openItems, func(i, j int) bool {
		return coverageWorkItemFocusPriority(openItems[i]) < coverageWorkItemFocusPriority(openItems[j])
	})
	for idx, item := range openItems {
		if idx == 0 {
			currentFocus = coverageWorkItemFocus(item)
		}
		if len(currentFocusItems) < 3 {
			focus := coverageWorkItemFocus(item)
			currentFocusItems = append(currentFocusItems, focus)
			focusIDs[stringFromMap(focus, "id")] = true
		}
	}
	if currentFocus == nil {
		for _, item := range items {
			if strings.TrimSpace(stringFromMap(item, "status")) == "blocked" {
				currentFocus = coverageWorkItemFocus(item)
				if len(currentFocusItems) == 0 {
					currentFocusItems = append(currentFocusItems, currentFocus)
					focusIDs[stringFromMap(currentFocus, "id")] = true
				}
				break
			}
		}
	}
	omittedCount := 0
	omittedByStatus := map[string]any{}
	omittedBySourceScope := map[string]any{}
	for _, item := range items {
		summaryItem := coverageWorkItemLedgerSummary(item, focusIDs[stringFromMap(item, "id")])
		ledgerSummary = append(ledgerSummary, summaryItem)
		if boolValue(summaryItem["omittedFromCurrentFocus"]) {
			omittedCount++
			status := firstNonEmptyString(stringFromMap(item, "status"), "open")
			scope := stringFromMap(item, "sourceScope")
			omittedByStatus[status] = intFromAny(omittedByStatus[status]) + 1
			omittedBySourceScope[scope] = intFromAny(omittedBySourceScope[scope]) + 1
		}
	}
	context := map[string]any{
		"schemaVersion":                  coverageWorkItemContextSchema,
		"definitionRevision":             revision,
		"coverageBaselineHash":           "sha256:" + coverageShortHash(items),
		"summary":                        summary,
		"unresolvedItems":                unresolved,
		"currentFocusItems":              currentFocusItems,
		"ledgerSummary":                  ledgerSummary,
		"omittedCount":                   omittedCount,
		"omittedStatusDistribution":      omittedByStatus,
		"omittedSourceScopeDistribution": omittedBySourceScope,
		"fullLedgerRef":                  "run.executionContext.coverageWorkItems",
	}
	if len(excludedSources) > 0 {
		context["excludedSources"] = excludedSources
		context["excludedCount"] = len(excludedSources)
	}
	if len(historicalItems) > 0 {
		context["historicalItems"] = historicalItems
		context["historicalCount"] = len(historicalItems)
	}
	if currentFocus != nil {
		context["currentFocus"] = currentFocus
	}
	return context
}

func coverageWorkItemFocusPriority(item map[string]any) int {
	if len(item) == 0 {
		return 100
	}
	missing := stringSet(coverageMissingEvidenceKinds(item))
	status := strings.TrimSpace(stringFromMap(item, "status"))
	if len(missing) == 0 && status == "evidence_ready" {
		return 4
	}
	switch {
	case status == "failed" && missing["state_change"]:
		return 0
	case missing["state_change"]:
		return 1
	case status == "failed" || len(stringSliceFromValue(item["rejectedEvidenceRefs"])) > 0:
		return 2
	case missing["validation"] || missing["artifact"]:
		return 3
	default:
		return 4
	}
}

func coverageWorkItemClosedForFocus(status string) bool {
	switch strings.TrimSpace(status) {
	case "satisfied", "blocked", "skipped":
		return true
	default:
		return false
	}
}

func coverageWorkItemFocus(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	focus := coverageWorkItemSummary(item)
	focus["acceptedEvidenceRefs"] = stringSliceFromValue(item["acceptedEvidenceRefs"])
	focus["rejectedEvidenceRefs"] = stringSliceFromValue(item["rejectedEvidenceRefs"])
	focus["acceptedEvidenceByKind"] = mapStringSliceValue(item["acceptedEvidenceByKind"])
	focus["rejectedEvidenceByKind"] = mapStringSliceValue(item["rejectedEvidenceByKind"])
	focus["coverageBoundary"] = stringFromMap(item, "coverageBoundary")
	focus["nextAllowedConvergenceStates"] = coverageWorkItemNextStates(item)
	return focus
}

func coverageWorkItemSummary(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	return map[string]any{
		"id":                       stringFromMap(item, "id"),
		"source":                   stringFromMap(item, "source"),
		"sourceScope":              stringFromMap(item, "sourceScope"),
		"evidenceRole":             stringFromMap(item, "evidenceRole"),
		"definitionRevision":       stringFromMap(item, "definitionRevision"),
		"sourceRequirementId":      stringFromMap(item, "sourceRequirementId"),
		"sourceRequirementAliases": stringSliceFromValue(item["sourceRequirementAliases"]),
		"sourcePath":               stringFromMap(item, "sourcePath"),
		"targetText":               stringFromMap(item, "targetText"),
		"normalizedSourceKey":      stringFromMap(item, "normalizedSourceKey"),
		"requiredEvidenceKinds":    stringSliceFromValue(item["requiredEvidenceKinds"]),
		"missingEvidenceKinds":     coverageMissingEvidenceKinds(item),
		"status":                   firstNonEmptyString(stringFromMap(item, "status"), "open"),
		"currentGap":               stringFromMap(item, "currentGap"),
	}
}

func coverageWorkItemNextStates(item map[string]any) []string {
	missing := stringSet(coverageMissingEvidenceKinds(item))
	if strings.TrimSpace(stringFromMap(item, "status")) == "failed" {
		switch {
		case missing["state_change"]:
			return []string{convergenceStateNeedEvidence, convergenceStateNeedStateChange}
		case missing["validation"] || missing["artifact"] || len(stringSliceFromValue(item["rejectedEvidenceRefs"])) > 0:
			return []string{convergenceStateNeedValidation}
		default:
			return []string{convergenceStateNeedValidation, convergenceStateNeedAssessment}
		}
	}
	if missing["external_dependency"] || missing["decision"] || missing["approval"] {
		return []string{convergenceStateNeedGovernance}
	}
	if missing["state_change"] {
		return []string{convergenceStateNeedEvidence, convergenceStateNeedStateChange}
	}
	if missing["validation"] || missing["artifact"] {
		return []string{convergenceStateNeedValidation}
	}
	return []string{convergenceStateNeedAssessment}
}

func coverageWorkItemLedgerSummary(item map[string]any, inFocus bool) map[string]any {
	ret := map[string]any{
		"id":                       stringFromMap(item, "id"),
		"sourceScope":              stringFromMap(item, "sourceScope"),
		"evidenceRole":             stringFromMap(item, "evidenceRole"),
		"sourceRequirementId":      stringFromMap(item, "sourceRequirementId"),
		"sourceRequirementAliases": stringSliceFromValue(item["sourceRequirementAliases"]),
		"status":                   firstNonEmptyString(stringFromMap(item, "status"), "open"),
		"missingEvidenceKinds":     coverageMissingEvidenceKinds(item),
		"acceptedEvidenceRefs":     latestCoverageEvidenceRefs(stringSliceFromValue(item["acceptedEvidenceRefs"]), 2),
		"rejectedEvidenceRefs":     latestCoverageEvidenceRefs(stringSliceFromValue(item["rejectedEvidenceRefs"]), 2),
		"omittedFromCurrentFocus":  !inFocus,
	}
	if !inFocus {
		ret["omittedReason"] = "context_budget_top_k"
	}
	return ret
}

func coverageExcludedSourcesFromContext(executionContext map[string]any) []map[string]any {
	return mapSliceFromValue(mapFromValue(executionContext[coverageWorkItemContextKey])["excludedSources"])
}

func coverageHistoricalItemsFromContext(executionContext map[string]any) []map[string]any {
	return mapSliceFromValue(mapFromValue(executionContext[coverageWorkItemContextKey])["historicalItems"])
}

func latestCoverageEvidenceRefs(refs []string, limit int) []string {
	refs = compactUniqueStrings(refs)
	if limit <= 0 || len(refs) <= limit {
		return refs
	}
	return refs[len(refs)-limit:]
}

func coverageWorkItemContextFromInput(input map[string]any) map[string]any {
	for _, source := range []map[string]any{
		input,
		mapFromValue(input["executionContext"]),
		mapFromValue(input["executionContextPackage"]),
		mapFromValue(input["repairActionPackage"]),
	} {
		if context := mapFromValue(source[coverageWorkItemContextKey]); len(context) > 0 {
			return context
		}
	}
	return nil
}

func coverageWorkItemPrimaryFocus(context map[string]any) map[string]any {
	if focus := mapFromValue(context["currentFocus"]); len(focus) > 0 {
		return focus
	}
	for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
		if len(item) > 0 {
			return item
		}
	}
	return nil
}

func coverageTargetSetFromWorkItemContext(context map[string]any) []map[string]any {
	if len(context) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]map[string]any, 0)
	add := func(item map[string]any) {
		if len(item) == 0 {
			return
		}
		targetText := strings.TrimSpace(stringFromMap(item, "targetText"))
		if targetText == "" {
			return
		}
		sourceRequirementID := strings.TrimSpace(stringFromMap(item, "sourceRequirementId"))
		key := firstNonEmptyString(sourceRequirementID, targetText)
		key = stringFromMap(item, "sourceScope") + ":" + normalizeCoverageGapRepairToken(key)
		if key == ":" || seen[key] {
			return
		}
		seen[key] = true
		target := map[string]any{
			"kind":                coverageWorkItemTargetKind(item),
			"targetText":          targetText,
			"state":               firstNonEmptyString(stringFromMap(item, "status"), "open"),
			"source":              stringFromMap(item, "source"),
			"sourceScope":         stringFromMap(item, "sourceScope"),
			"sourceRequirementId": sourceRequirementID,
			"sourcePath":          stringFromMap(item, "sourcePath"),
			"coverageWorkItemId":  stringFromMap(item, "id"),
			"coverageTargetToken": firstNonEmptyString(sourceRequirementID, targetText),
		}
		ret = append(ret, target)
	}
	if focus := mapFromValue(context["currentFocus"]); len(focus) > 0 {
		add(focus)
	}
	for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
		add(item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func coverageWorkItemTargetKind(item map[string]any) string {
	switch strings.TrimSpace(stringFromMap(item, "sourceScope")) {
	case "goal_leaf":
		return "goal"
	case "dod_item":
		return "dod"
	case "deliverable":
		return "deliverable"
	default:
		return firstNonEmptyString(stringFromMap(item, "source"), stringFromMap(item, "sourceScope"))
	}
}

func applyCoverageWorkItemContext(input map[string]any, context map[string]any) {
	if len(input) == 0 || len(context) == 0 {
		return
	}
	input[coverageWorkItemContextKey] = context
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		pkg[coverageWorkItemContextKey] = context
		input["repairActionPackage"] = pkg
	}
}

func attachCoverageWorkItemFailureDiagnostics(run *runModel.Run, step *runModel.Step) {
	if run == nil || step == nil || len(step.Output) == 0 {
		return
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	if len(context) == 0 {
		return
	}
	unresolved := coverageWorkItemUnresolvedItemsFromContext(context)
	if len(unresolved) == 0 {
		return
	}
	step.Output[coverageWorkItemContextKey] = context
	step.Output["unresolvedCoverageWorkItems"] = unresolved
	diagnostics := mapFromValue(step.Output["runtimeDiagnostics"])
	if diagnostics == nil {
		diagnostics = map[string]any{}
	}
	diagnostics["coverageWorkItemContext"] = context
	diagnostics["unresolvedCoverageWorkItems"] = unresolved
	step.Output["runtimeDiagnostics"] = diagnostics
}

func coverageWorkItemUnresolvedItemsFromContext(context map[string]any) []map[string]any {
	if len(context) == 0 {
		return nil
	}
	if unresolved := mapSliceFromValue(context["unresolvedItems"]); len(unresolved) > 0 {
		return unresolved
	}
	seen := map[string]bool{}
	ret := make([]map[string]any, 0)
	add := func(item map[string]any) {
		if len(item) == 0 || coverageWorkItemClosedForFocus(stringFromMap(item, "status")) {
			return
		}
		id := strings.TrimSpace(stringFromMap(item, "id"))
		key := firstNonEmptyString(id, stringFromMap(item, "sourceRequirementId"), stringFromMap(item, "targetText"))
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		ret = append(ret, coverageWorkItemSummary(item))
	}
	add(mapFromValue(context["currentFocus"]))
	for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
		add(item)
	}
	return ret
}
