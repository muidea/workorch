package orchestrator

import (
	"fmt"
	"sort"
	"strings"
)

const (
	acceptanceSlicesKey     = "acceptanceSlices"
	activeSliceIDKey        = "activeSliceId"
	coverageFocusKey        = "coverageFocus"
	convergenceBudgetKey    = "convergenceBudget"
	acceptancePolicyKey     = "acceptanceSlicePolicy"
	acceptancePolicyVersion = "workorch.acceptance_slice_policy.v1"
)

type acceptanceSliceGroupPolicy struct {
	ID                       string
	Title                    string
	Scope                    string
	Priority                 int
	MaxWorkItems             int
	CompletionPolicy         string
	ResultPolicy             string
	RequiredEvidenceKinds    []string
	MatchAll                 bool
	MatchSources             []string
	MatchSourceScopes        []string
	MatchEvidenceRoles       []string
	MatchRequiredKinds       []string
	MatchMissingKinds        []string
	MatchSourceRequirementID []string
}

type acceptanceSlicePolicy struct {
	SchemaVersion    string
	MaxWorkItems     int
	CompletionPolicy string
	ResultPolicy     string
	Groups           []acceptanceSliceGroupPolicy
	Budget           map[string]any
}

func synchronizeAcceptanceProjection(executionContext map[string]any, items []map[string]any, coverageContext map[string]any) bool {
	if executionContext == nil {
		return false
	}
	if len(items) == 0 {
		changed := false
		for _, key := range []string{acceptanceSlicesKey, activeSliceIDKey, coverageFocusKey, convergenceBudgetKey} {
			if _, ok := executionContext[key]; ok {
				delete(executionContext, key)
				changed = true
			}
		}
		return changed
	}
	policy := acceptancePolicyFromExecutionContext(executionContext)
	slices := buildAcceptanceSlicesFromCoverageItems(items, policy)
	activeSliceID := selectActiveAcceptanceSliceID(slices, strings.TrimSpace(stringFromMap(executionContext, activeSliceIDKey)))
	focus := buildCoverageFocusForActiveSlice(activeSliceID, slices, items, coverageContext)
	budget := convergenceBudgetFromPolicy(policy, mapFromValue(executionContext[convergenceBudgetKey]))

	changed := false
	if stableCoverageJSON(mapSliceFromValue(executionContext[acceptanceSlicesKey])) != stableCoverageJSON(slices) {
		executionContext[acceptanceSlicesKey] = slices
		changed = true
	}
	if strings.TrimSpace(stringFromMap(executionContext, activeSliceIDKey)) != activeSliceID {
		if activeSliceID == "" {
			delete(executionContext, activeSliceIDKey)
		} else {
			executionContext[activeSliceIDKey] = activeSliceID
		}
		changed = true
	}
	if stableCoverageJSON(mapFromValue(executionContext[coverageFocusKey])) != stableCoverageJSON(focus) {
		if len(focus) == 0 {
			delete(executionContext, coverageFocusKey)
		} else {
			executionContext[coverageFocusKey] = focus
		}
		changed = true
	}
	if stableCoverageJSON(mapFromValue(executionContext[convergenceBudgetKey])) != stableCoverageJSON(budget) {
		if len(budget) == 0 {
			delete(executionContext, convergenceBudgetKey)
		} else {
			executionContext[convergenceBudgetKey] = budget
		}
		changed = true
	}
	return changed
}

func acceptancePolicyFromExecutionContext(executionContext map[string]any) acceptanceSlicePolicy {
	source := mapFromValue(executionContext[acceptancePolicyKey])
	if len(source) == 0 {
		source = mapFromValue(mapFromValue(executionContext["executionContract"])[acceptancePolicyKey])
	}
	policy := acceptanceSlicePolicy{
		SchemaVersion:    firstNonEmptyString(stringFromMap(source, "schemaVersion"), acceptancePolicyVersion),
		MaxWorkItems:     positiveIntFromValue(source["maxWorkItems"], 5),
		CompletionPolicy: firstNonEmptyString(stringFromMap(source, "completionPolicy"), "all_required_items_accepted"),
		ResultPolicy:     firstNonEmptyString(stringFromMap(source, "resultPolicy"), "slice_result_allowed"),
		Budget:           cloneStepPayload(mapFromValue(source["budget"])),
	}
	for _, group := range mapSliceFromValue(firstNonNil(source["groups"], source["slices"])) {
		id := strings.TrimSpace(stringFromMap(group, "id"))
		if id == "" {
			continue
		}
		match := mapFromValue(group["match"])
		policy.Groups = append(policy.Groups, acceptanceSliceGroupPolicy{
			ID:                       id,
			Title:                    firstNonEmptyString(stringFromMap(group, "title"), id),
			Scope:                    firstNonEmptyString(stringFromMap(group, "scope"), id),
			Priority:                 int(positiveIntFromValue(group["priority"], 100)),
			MaxWorkItems:             int(positiveIntFromValue(firstNonNil(group["maxWorkItems"], source["maxWorkItems"]), int64(policy.MaxWorkItems))),
			CompletionPolicy:         firstNonEmptyString(stringFromMap(group, "completionPolicy"), policy.CompletionPolicy),
			ResultPolicy:             firstNonEmptyString(stringFromMap(group, "resultPolicy"), policy.ResultPolicy),
			RequiredEvidenceKinds:    compactUniqueStrings(stringSliceFromValue(firstNonNil(group["requiredEvidenceKinds"], match["requiredEvidenceKinds"]))),
			MatchAll:                 boolValue(match["all"]),
			MatchSources:             compactUniqueStrings(stringSliceFromValue(match["sources"])),
			MatchSourceScopes:        compactUniqueStrings(stringSliceFromValue(match["sourceScopes"])),
			MatchEvidenceRoles:       compactUniqueStrings(stringSliceFromValue(match["evidenceRoles"])),
			MatchRequiredKinds:       compactUniqueStrings(stringSliceFromValue(match["requiredEvidenceKinds"])),
			MatchMissingKinds:        compactUniqueStrings(stringSliceFromValue(match["missingEvidenceKinds"])),
			MatchSourceRequirementID: compactUniqueStrings(stringSliceFromValue(match["sourceRequirementIds"])),
		})
	}
	sort.SliceStable(policy.Groups, func(i, j int) bool {
		if policy.Groups[i].Priority != policy.Groups[j].Priority {
			return policy.Groups[i].Priority < policy.Groups[j].Priority
		}
		return policy.Groups[i].ID < policy.Groups[j].ID
	})
	return policy
}

func buildAcceptanceSlicesFromCoverageItems(items []map[string]any, policy acceptanceSlicePolicy) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	assigned := map[string]bool{}
	var slices []map[string]any
	for _, group := range policy.Groups {
		var matched []map[string]any
		for _, item := range items {
			id := strings.TrimSpace(stringFromMap(item, "id"))
			if id == "" || assigned[id] || !acceptanceSliceGroupMatchesItem(group, item) {
				continue
			}
			matched = append(matched, item)
			assigned[id] = true
		}
		slices = append(slices, acceptanceSlicesForGroup(group.ID, group.Title, group.Scope, group.Priority, group.MaxWorkItems, group.CompletionPolicy, group.ResultPolicy, group.RequiredEvidenceKinds, matched)...)
	}
	var remaining []map[string]any
	for _, item := range items {
		id := strings.TrimSpace(stringFromMap(item, "id"))
		if id == "" || assigned[id] {
			continue
		}
		remaining = append(remaining, item)
	}
	if len(remaining) > 0 {
		group := acceptanceSliceGroupPolicy{
			ID:               "slice-required",
			Title:            "Required acceptance",
			Scope:            "required",
			Priority:         100,
			MaxWorkItems:     policy.MaxWorkItems,
			CompletionPolicy: policy.CompletionPolicy,
			ResultPolicy:     policy.ResultPolicy,
		}
		slices = append(slices, acceptanceSlicesForGroup(group.ID, group.Title, group.Scope, group.Priority, group.MaxWorkItems, group.CompletionPolicy, group.ResultPolicy, nil, remaining)...)
	}
	return slices
}

func acceptanceSlicesForGroup(id, title, scope string, priority, maxItems int, completionPolicy, resultPolicy string, requiredEvidenceKinds []string, items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	if maxItems <= 0 {
		maxItems = 5
	}
	var ret []map[string]any
	for start, ordinal := 0, 1; start < len(items); start, ordinal = start+maxItems, ordinal+1 {
		end := start + maxItems
		if end > len(items) {
			end = len(items)
		}
		chunk := items[start:end]
		sliceID := id
		if len(items) > maxItems {
			sliceID = fmt.Sprintf("%s-%03d", id, ordinal)
		}
		ret = append(ret, map[string]any{
			"id":                    sliceID,
			"title":                 title,
			"state":                 acceptanceSliceState(chunk),
			"priority":              priority + ordinal - 1,
			"scope":                 scope,
			"coverageWorkItemIds":   coverageWorkItemIDs(chunk),
			"requiredEvidenceKinds": acceptanceSliceRequiredEvidenceKinds(chunk, requiredEvidenceKinds),
			"completionPolicy":      firstNonEmptyString(completionPolicy, "all_required_items_accepted"),
			"resultPolicy":          firstNonEmptyString(resultPolicy, "slice_result_allowed"),
		})
	}
	return ret
}

func acceptanceSliceGroupMatchesItem(group acceptanceSliceGroupPolicy, item map[string]any) bool {
	if group.MatchAll {
		return true
	}
	if len(group.MatchSources) > 0 && !stringSet(group.MatchSources)[stringFromMap(item, "source")] {
		return false
	}
	if len(group.MatchSourceScopes) > 0 && !stringSet(group.MatchSourceScopes)[stringFromMap(item, "sourceScope")] {
		return false
	}
	if len(group.MatchEvidenceRoles) > 0 && !stringSet(group.MatchEvidenceRoles)[stringFromMap(item, "evidenceRole")] {
		return false
	}
	if len(group.MatchSourceRequirementID) > 0 && !stringSet(group.MatchSourceRequirementID)[stringFromMap(item, "sourceRequirementId")] {
		return false
	}
	if len(group.MatchRequiredKinds) > 0 && !stringSetsIntersect(group.MatchRequiredKinds, stringSliceFromValue(item["requiredEvidenceKinds"])) {
		return false
	}
	if len(group.MatchMissingKinds) > 0 && !stringSetsIntersect(group.MatchMissingKinds, coverageMissingEvidenceKinds(item)) {
		return false
	}
	return len(group.MatchSources) > 0 ||
		len(group.MatchSourceScopes) > 0 ||
		len(group.MatchEvidenceRoles) > 0 ||
		len(group.MatchSourceRequirementID) > 0 ||
		len(group.MatchRequiredKinds) > 0 ||
		len(group.MatchMissingKinds) > 0
}

func acceptanceSliceState(items []map[string]any) string {
	if len(items) == 0 {
		return "pending"
	}
	allAccepted := true
	allBlocked := true
	allFailed := true
	for _, item := range items {
		status := strings.TrimSpace(stringFromMap(item, "status"))
		if status != "satisfied" && status != "skipped" {
			allAccepted = false
		}
		if status != "blocked" {
			allBlocked = false
		}
		if status != "failed" {
			allFailed = false
		}
	}
	switch {
	case allAccepted:
		return "accepted"
	case allBlocked:
		return "blocked"
	case allFailed:
		return "failed"
	default:
		return "pending"
	}
}

func selectActiveAcceptanceSliceID(slices []map[string]any, previous string) string {
	if previous != "" {
		for _, item := range slices {
			if stringFromMap(item, "id") == previous && stringFromMap(item, "state") == "pending" {
				item["state"] = "active"
				return previous
			}
		}
	}
	for _, item := range slices {
		if stringFromMap(item, "state") == "pending" {
			item["state"] = "active"
			return stringFromMap(item, "id")
		}
	}
	return ""
}

func buildCoverageFocusForActiveSlice(activeSliceID string, slices []map[string]any, items []map[string]any, coverageContext map[string]any) map[string]any {
	if activeSliceID == "" {
		return nil
	}
	activeIDs := map[string]bool{}
	for _, slice := range slices {
		if stringFromMap(slice, "id") != activeSliceID {
			continue
		}
		for _, id := range stringSliceFromValue(slice["coverageWorkItemIds"]) {
			activeIDs[id] = true
		}
		break
	}
	if len(activeIDs) == 0 {
		return nil
	}
	itemByID := map[string]map[string]any{}
	for _, item := range items {
		id := stringFromMap(item, "id")
		if id != "" {
			itemByID[id] = item
		}
	}
	if focus := coverageWorkItemPrimaryFocus(coverageContext); len(focus) > 0 && activeIDs[stringFromMap(focus, "id")] {
		if item := itemByID[stringFromMap(focus, "id")]; len(item) > 0 {
			return coverageFocusFromWorkItem(activeSliceID, item)
		}
	}
	var candidates []map[string]any
	for id := range activeIDs {
		if item := itemByID[id]; len(item) > 0 && !coverageWorkItemClosedForFocus(stringFromMap(item, "status")) {
			candidates = append(candidates, item)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := coverageWorkItemFocusPriority(candidates[i])
		right := coverageWorkItemFocusPriority(candidates[j])
		if left != right {
			return left < right
		}
		return stringFromMap(candidates[i], "id") < stringFromMap(candidates[j], "id")
	})
	if len(candidates) == 0 {
		return nil
	}
	return coverageFocusFromWorkItem(activeSliceID, candidates[0])
}

func coverageFocusFromWorkItem(sliceID string, item map[string]any) map[string]any {
	id := stringFromMap(item, "id")
	if id == "" {
		return nil
	}
	return map[string]any{
		"id":                    "focus-" + id,
		"sliceId":               sliceID,
		"coverageWorkItemIds":   []string{id},
		"requiredEvidenceKinds": coverageMissingOrRequiredEvidenceKinds(item),
		"acceptedEvidenceRefs":  stringSliceFromValue(item["acceptedEvidenceRefs"]),
		"rejectedEvidenceRefs":  stringSliceFromValue(item["rejectedEvidenceRefs"]),
		"targetRefs":            acceptanceFocusTargetRefs(item),
		"state":                 "active",
	}
}

func convergenceBudgetFromPolicy(policy acceptanceSlicePolicy, existing map[string]any) map[string]any {
	budget := cloneStepPayload(existing)
	if budget == nil {
		budget = map[string]any{}
	}
	for key, value := range policy.Budget {
		budget[key] = cloneStepInputValue(value)
	}
	if strings.TrimSpace(stringFromMap(budget, "state")) == "" {
		budget["state"] = "tracking"
	}
	if strings.TrimSpace(stringFromMap(budget, "decision")) == "" {
		budget["decision"] = "continue"
	}
	for key, value := range defaultConvergenceBudgetLimits() {
		if positiveIntFromValue(budget[key], 0) == 0 {
			budget[key] = value
		}
	}
	return budget
}

func defaultConvergenceBudgetLimits() map[string]any {
	return map[string]any{
		"maxRepairAttemptsPerFocus":   4,
		"maxLLMRoundsPerSlice":        12,
		"maxRepeatedFailureSignature": 3,
		"maxValidationOnlyRetries":    3,
		"maxNoCoverageDeltaRounds":    3,
	}
}

func runtimeAcceptanceSignalFromExecutionContext(executionContext map[string]any) map[string]any {
	if len(executionContext) == 0 {
		return nil
	}
	ret := map[string]any{}
	activeSliceID := strings.TrimSpace(stringFromMap(executionContext, activeSliceIDKey))
	if activeSliceID != "" {
		ret["activeSliceId"] = activeSliceID
	}
	if active := activeAcceptanceSliceFromExecutionContext(executionContext, activeSliceID); len(active) > 0 {
		ret["activeSlice"] = active
	}
	if focus := mapFromValue(executionContext[coverageFocusKey]); len(focus) > 0 {
		ret["currentFocus"] = cloneStepPayload(focus)
	}
	if budget := mapFromValue(executionContext[convergenceBudgetKey]); len(budget) > 0 {
		ret["budget"] = cloneStepPayload(budget)
	}
	if len(ret) == 0 {
		return nil
	}
	ret["projectionPolicy"] = "dynamic C4 context; active slice and current focus constrain the next state-changing or validation output"
	return ret
}

func activeAcceptanceSliceFromExecutionContext(executionContext map[string]any, activeSliceID string) map[string]any {
	if activeSliceID == "" {
		return nil
	}
	for _, item := range mapSliceFromValue(executionContext[acceptanceSlicesKey]) {
		if stringFromMap(item, "id") == activeSliceID {
			return cloneStepPayload(item)
		}
	}
	return nil
}

func coverageWorkItemIDs(items []map[string]any) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if id := stringFromMap(item, "id"); id != "" {
			ret = append(ret, id)
		}
	}
	return ret
}

func acceptanceSliceRequiredEvidenceKinds(items []map[string]any, override []string) []string {
	if len(override) > 0 {
		return compactUniqueStrings(override)
	}
	var ret []string
	for _, item := range items {
		ret = append(ret, stringSliceFromValue(item["requiredEvidenceKinds"])...)
	}
	return compactUniqueStrings(ret)
}

func coverageMissingOrRequiredEvidenceKinds(item map[string]any) []string {
	if missing := coverageMissingEvidenceKinds(item); len(missing) > 0 {
		return missing
	}
	return stringSliceFromValue(item["requiredEvidenceKinds"])
}

func acceptanceFocusTargetRefs(item map[string]any) []string {
	return compactUniqueStrings([]string{
		stringFromMap(item, "sourcePath"),
		stringFromMap(item, "sourceRequirementId"),
		stringFromMap(item, "normalizedSourceKey"),
	})
}

func stringSetsIntersect(left []string, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	rightSet := stringSet(right)
	for _, item := range left {
		if rightSet[item] {
			return true
		}
	}
	return false
}

func positiveIntFromValue(value any, fallback int64) int {
	if parsed := int64FromValue(value); parsed > 0 {
		return int(parsed)
	}
	if fallback > 0 {
		return int(fallback)
	}
	return 0
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
