package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	coverageWorkItemsKey          = "coverageWorkItems"
	coverageWorkItemContextKey    = "coverageWorkItemContext"
	coverageWorkItemSchema        = "workorch.coverage_work_item.v1"
	coverageWorkItemContextSchema = "workorch.coverage_work_item_context.v1"
)

var coverageMarkdownLeafPattern = regexp.MustCompile(`^\s*(?:[-*+]\s+(?:\[[ xX]\]\s*)?|\d+[.)]\s+)(.+?)\s*$`)

type coverageWorkItemSource struct {
	Source              string
	SourceScope         string
	SourceGoalID        string
	SourceDoDID         string
	SourceRequirementID string
	SourcePath          string
	TargetText          string
	EvidenceRole        string
	ParentContext       string
	ExcludedReason      string
}

type coverageWorkItemUpdate struct {
	WorkItemIDs    []string
	TargetTokens   []string
	EvidenceKinds  []string
	EvidenceRefs   []string
	Accepted       bool
	CloseSatisfied bool
	Reason         string
}

func ensureRunCoverageWorkItems(run *runModel.Run) bool {
	if run == nil {
		return false
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	items, context := buildCoverageWorkItemsForRun(run)
	currentItems := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	currentContext := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	if len(items) == 0 && len(context) == 0 {
		if _, ok := run.ExecutionContext[coverageWorkItemsKey]; ok {
			delete(run.ExecutionContext, coverageWorkItemsKey)
			delete(run.ExecutionContext, coverageWorkItemContextKey)
			return true
		}
		if _, ok := run.ExecutionContext[coverageWorkItemContextKey]; ok {
			delete(run.ExecutionContext, coverageWorkItemContextKey)
			return true
		}
		return false
	}
	if stableCoverageJSON(currentItems) == stableCoverageJSON(items) &&
		stableCoverageJSON(currentContext) == stableCoverageJSON(context) {
		return false
	}
	run.ExecutionContext[coverageWorkItemsKey] = items
	run.ExecutionContext[coverageWorkItemContextKey] = context
	return true
}

func prepareCoverageWorkItemsForReplanSuccessor(run *runModel.Run) bool {
	if run == nil {
		return false
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	return ensureRunCoverageWorkItems(run)
}

func updateRunCoverageWorkItemsFromCompletedStep(run *runModel.Run, step *runModel.Step) bool {
	if run == nil || step == nil || step.State != common.StepStateCompleted {
		return false
	}
	ensureRunCoverageWorkItems(run)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return false
	}
	update := coverageWorkItemStepUpdate(step)
	if len(update.EvidenceKinds) == 0 || (len(update.WorkItemIDs) == 0 && len(update.TargetTokens) == 0) {
		return false
	}
	changed := false
	for _, item := range items {
		if !coverageWorkItemMatchesUpdate(item, update) {
			continue
		}
		if coverageApplyWorkItemEvidence(item, update.EvidenceKinds, update.EvidenceRefs, true, false, string(step.ID), "") {
			changed = true
		}
	}
	if !changed {
		return false
	}
	revision := coverageDefinitionRevision(run.ExecutionContext)
	run.ExecutionContext[coverageWorkItemsKey] = items
	run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(revision, items, coverageExcludedSourcesFromContext(run.ExecutionContext), coverageHistoricalItemsFromContext(run.ExecutionContext))
	return true
}

func updateRunCoverageWorkItemsFromFailedStep(run *runModel.Run, step *runModel.Step, reason string) bool {
	if run == nil || step == nil || step.State != common.StepStateFailed {
		return false
	}
	ensureRunCoverageWorkItems(run)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return false
	}
	update := coverageWorkItemStepUpdate(step)
	update.Accepted = false
	update.Reason = firstNonEmptyString(strings.TrimSpace(reason), stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason"))
	if len(update.EvidenceKinds) == 0 || (len(update.WorkItemIDs) == 0 && len(update.TargetTokens) == 0) {
		return false
	}
	changed := false
	for _, item := range items {
		if !coverageWorkItemMatchesUpdate(item, update) {
			continue
		}
		if coverageApplyWorkItemEvidence(item, update.EvidenceKinds, update.EvidenceRefs, false, false, string(step.ID), update.Reason) {
			changed = true
		}
	}
	if !changed {
		return false
	}
	revision := coverageDefinitionRevision(run.ExecutionContext)
	run.ExecutionContext[coverageWorkItemsKey] = items
	run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(revision, items, coverageExcludedSourcesFromContext(run.ExecutionContext), coverageHistoricalItemsFromContext(run.ExecutionContext))
	return true
}

func updateRunCoverageWorkItemsFromAssessment(run *runModel.Run, step *runModel.Step, assessment map[string]any) bool {
	if run == nil || step == nil || len(assessment) == 0 {
		return false
	}
	ensureRunCoverageWorkItems(run)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return false
	}
	updates := coverageWorkItemAssessmentUpdates(step, assessment)
	if len(updates) == 0 {
		return false
	}
	changed := false
	matchedAny := false
	var unmatchedAccepted []coverageWorkItemUpdate
	for _, update := range updates {
		matched := false
		for _, item := range items {
			if !coverageWorkItemMatchesUpdate(item, update) {
				continue
			}
			matched = true
			matchedAny = true
			if coverageApplyWorkItemEvidence(item, update.EvidenceKinds, update.EvidenceRefs, update.Accepted, update.CloseSatisfied, string(step.ID), update.Reason) {
				changed = true
			}
		}
		if !matched && update.Accepted {
			unmatchedAccepted = append(unmatchedAccepted, update)
		}
	}
	if !matchedAny && len(unmatchedAccepted) > 0 {
		if coverageApplyUnlinkedAssessmentRejectedEvidence(items, unmatchedAccepted, step) {
			changed = true
		}
	}
	if !changed {
		return false
	}
	revision := coverageDefinitionRevision(run.ExecutionContext)
	run.ExecutionContext[coverageWorkItemsKey] = items
	run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(revision, items, coverageExcludedSourcesFromContext(run.ExecutionContext), coverageHistoricalItemsFromContext(run.ExecutionContext))
	return true
}

func coverageApplyUnlinkedAssessmentRejectedEvidence(items []map[string]any, updates []coverageWorkItemUpdate, step *runModel.Step) bool {
	if len(items) == 0 || len(updates) == 0 || step == nil {
		return false
	}
	var focus map[string]any
	for _, item := range items {
		status := strings.TrimSpace(stringFromMap(item, "status"))
		if status != "satisfied" && status != "blocked" && status != "skipped" {
			focus = item
			break
		}
	}
	if len(focus) == 0 {
		return false
	}
	refs := make([]string, 0)
	for _, update := range updates {
		refs = append(refs, update.EvidenceRefs...)
	}
	if len(refs) == 0 {
		refs = []string{string(step.ID)}
	}
	reason := "satisfied assessment item did not relink to any active coverage work item"
	return coverageApplyWorkItemEvidence(focus, []string{"validation"}, refs, false, false, string(step.ID), reason)
}

func validateRunCoverageWorkItemsReadyForAcceptance(run *runModel.Run) error {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	ensureRunCoverageWorkItems(run)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return nil
	}
	var open []string
	var unlinked []string
	for _, item := range items {
		status := strings.TrimSpace(stringFromMap(item, "status"))
		switch status {
		case "satisfied", "blocked", "skipped":
			continue
		}
		missing := coverageMissingEvidenceKinds(item)
		if len(missing) == 0 && status == "evidence_ready" {
			open = append(open, fmt.Sprintf("%s:assessment_pending", stringFromMap(item, "id")))
			continue
		}
		if len(missing) > 0 {
			open = append(open, fmt.Sprintf("%s:%s", stringFromMap(item, "id"), strings.Join(missing, ",")))
			continue
		}
		open = append(open, fmt.Sprintf("%s:%s", stringFromMap(item, "id"), firstNonEmptyString(status, "open")))
	}
	for _, item := range items {
		status := strings.TrimSpace(stringFromMap(item, "status"))
		if status != "satisfied" {
			continue
		}
		if len(stringSliceFromValue(item["acceptedEvidenceRefs"])) == 0 {
			unlinked = append(unlinked, stringFromMap(item, "id"))
		}
	}
	if len(open) > 0 {
		return fmt.Errorf("%w: coverageWorkItems not ready for accepted result; unresolved: %s", common.ErrInvalidArgument, strings.Join(limitedStrings(open, 8), ", "))
	}
	if len(unlinked) > 0 {
		return fmt.Errorf("%w: coverageWorkItems satisfied without accepted evidence refs: %s", common.ErrInvalidArgument, strings.Join(limitedStrings(unlinked, 8), ", "))
	}
	return nil
}

func buildCoverageWorkItemsForRun(run *runModel.Run) ([]map[string]any, map[string]any) {
	if run == nil {
		return nil, nil
	}
	executionContext := cloneStepPayload(run.ExecutionContext)
	if len(executionContext) == 0 {
		return nil, nil
	}
	revision := coverageDefinitionRevision(executionContext)
	sources, excludedSources := coverageWorkItemSources(executionContext)
	if len(sources) == 0 && len(excludedSources) == 0 {
		return nil, nil
	}
	existingByKey := map[string]map[string]any{}
	historicalItems := coverageHistoricalItemsFromContext(executionContext)
	for _, item := range mapSliceFromValue(executionContext[coverageWorkItemsKey]) {
		if strings.TrimSpace(stringFromMap(item, "definitionRevision")) != revision {
			historicalItems = append(historicalItems, coverageHistoricalWorkItemSummary(item, "superseded_definition_revision"))
			continue
		}
		key := strings.TrimSpace(stringFromMap(item, "normalizedSourceKey"))
		if key != "" {
			existingByKey[key] = item
		}
	}
	items := make([]map[string]any, 0, len(sources))
	seen := map[string]bool{}
	for _, source := range sources {
		key := coverageWorkItemSourceKey(source)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		item := coverageWorkItemFromSource(len(items)+1, revision, key, source, existingByKey[key], run)
		items = append(items, item)
	}
	supersededKeys := make([]string, 0, len(existingByKey))
	for key := range existingByKey {
		if !seen[key] {
			supersededKeys = append(supersededKeys, key)
		}
	}
	sort.Strings(supersededKeys)
	for _, key := range supersededKeys {
		historicalItems = append(historicalItems, coverageHistoricalWorkItemSummary(existingByKey[key], "superseded_coverage_source"))
	}
	context := coverageWorkItemContextFromItems(revision, items, excludedSources, historicalItems)
	return items, context
}

func coverageDefinitionRevision(executionContext map[string]any) string {
	for _, key := range []string{"definitionRevision", "goalDoDRevision", "goalRevision", "doDRevision"} {
		if value := strings.TrimSpace(stringFromMap(executionContext, key)); value != "" {
			return value
		}
	}
	if impact := mapFromValue(executionContext["definitionChangeImpact"]); len(impact) > 0 {
		for _, key := range []string{"definitionRevision", "currentRevision", "revision"} {
			if value := strings.TrimSpace(stringFromMap(impact, key)); value != "" {
				return value
			}
		}
	}
	payload := map[string]any{
		"goal":              executionContext["goal"],
		"goals":             executionContext["goals"],
		"dod":               executionContext["dod"],
		"dods":              executionContext["dods"],
		"executionContract": executionContext["executionContract"],
	}
	return "rev-" + coverageShortHash(payload)
}

func coverageWorkItemSources(executionContext map[string]any) ([]coverageWorkItemSource, []map[string]any) {
	sources := make([]coverageWorkItemSource, 0)
	excluded := make([]map[string]any, 0)
	addExcluded := func(source, scope, path, text, reason string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		excluded = append(excluded, map[string]any{
			"source":         source,
			"sourceScope":    scope,
			"sourcePath":     path,
			"targetText":     text,
			"excludedReason": reason,
		})
	}
	addGoal := func(goal map[string]any, pathPrefix string) {
		if len(goal) == 0 {
			return
		}
		goalID := strings.TrimSpace(stringFromMap(goal, "id"))
		for idx, leaf := range coverageLeafTexts(stringFromMap(goal, "text")) {
			sources = append(sources, coverageWorkItemSource{
				Source:              "goal",
				SourceScope:         "goal_leaf",
				SourceGoalID:        goalID,
				SourceRequirementID: fmt.Sprintf("%sgoal-leaf-%03d", coverageRequirementPrefix(goalID), idx+1),
				SourcePath:          fmt.Sprintf("%sgoal.leaves[%d]", pathPrefix, idx),
				TargetText:          leaf["text"],
				ParentContext:       leaf["parent"],
			})
		}
	}
	addDoD := func(dod map[string]any, pathPrefix string) {
		if len(dod) == 0 {
			return
		}
		dodID := strings.TrimSpace(stringFromMap(dod, "id"))
		for idx, item := range stringSliceFromValue(dod["items"]) {
			item = strings.TrimSpace(item)
			if item == "" || coverageTextLooksContainer(item) {
				if item != "" {
					addExcluded("dod", "dod_item", fmt.Sprintf("%sdod.items[%d]", pathPrefix, idx), item, "container_source")
				}
				continue
			}
			sources = append(sources, coverageWorkItemSource{
				Source:              "dod",
				SourceScope:         "dod_item",
				SourceDoDID:         dodID,
				SourceRequirementID: fmt.Sprintf("%sdod-item-%03d", coverageRequirementPrefix(dodID), idx+1),
				SourcePath:          fmt.Sprintf("%sdod.items[%d]", pathPrefix, idx),
				TargetText:          item,
			})
		}
	}
	addGoal(mapFromValue(executionContext["goal"]), "")
	for idx, goal := range mapSliceFromValue(executionContext["goals"]) {
		addGoal(goal, fmt.Sprintf("goals[%d].", idx))
	}
	addDoD(mapFromValue(executionContext["dod"]), "")
	for idx, dod := range mapSliceFromValue(executionContext["dods"]) {
		addDoD(dod, fmt.Sprintf("dods[%d].", idx))
	}
	sources = append(sources, profileCoverageWorkItemSources(executionContext)...)
	sources = append(sources, failedCoverageWorkItemSources(executionContext)...)
	return sources, excluded
}

func profileCoverageWorkItemSources(executionContext map[string]any) []coverageWorkItemSource {
	executionContract := mapFromValue(executionContext["executionContract"])
	profileContracts := mapFromValue(executionContract["profileContracts"])
	if len(profileContracts) == 0 {
		return nil
	}
	profileNames := make([]string, 0, len(profileContracts))
	for name := range profileContracts {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)
	ret := make([]coverageWorkItemSource, 0)
	for _, profileName := range profileNames {
		profile := mapFromValue(profileContracts[profileName])
		for idx, item := range mapSliceFromValue(profile["acceptanceDimensions"]) {
			if !boolValue(item["required"]) {
				continue
			}
			id := firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "dimension"), stringFromMap(item, "key"), stringFromMap(item, "name"))
			text := firstNonEmptyString(id, stringFromMap(item, "description"))
			if strings.TrimSpace(text) == "" {
				continue
			}
			ret = append(ret, coverageWorkItemSource{
				Source:              "acceptance_dimension",
				SourceScope:         "acceptance_dimension",
				SourceRequirementID: id,
				SourcePath:          fmt.Sprintf("executionContract.profileContracts.%s.acceptanceDimensions[%d]", profileName, idx),
				TargetText:          text,
				EvidenceRole:        normalizedCoverageEvidenceRole(stringFromMap(item, "evidenceRole")),
			})
		}
		for idx, item := range mapSliceFromValue(profile["deliverableExpectations"]) {
			if !boolValue(item["required"]) {
				continue
			}
			id := firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "deliverable"), stringFromMap(item, "name"), stringFromMap(item, "path"))
			text := firstNonEmptyString(id, stringFromMap(item, "description"))
			if strings.TrimSpace(text) == "" {
				continue
			}
			ret = append(ret, coverageWorkItemSource{
				Source:              "deliverable",
				SourceScope:         "deliverable",
				SourceRequirementID: id,
				SourcePath:          fmt.Sprintf("executionContract.profileContracts.%s.deliverableExpectations[%d]", profileName, idx),
				TargetText:          text,
				EvidenceRole:        normalizedCoverageEvidenceRole(stringFromMap(item, "evidenceRole")),
			})
		}
	}
	return ret
}

func failedCoverageWorkItemSources(executionContext map[string]any) []coverageWorkItemSource {
	var ret []coverageWorkItemSource
	for _, item := range mapSliceFromValue(executionContext["missingDoDItems"]) {
		if source := failedCoverageSourceFromItem("dod", "dod_item", "missingDoDItems", item); source.TargetText != "" {
			ret = append(ret, source)
		}
	}
	for _, item := range mapSliceFromValue(executionContext["missingGoalCoverage"]) {
		if source := failedCoverageSourceFromItem("goal", "goal_leaf", "missingGoalCoverage", item); source.TargetText != "" {
			ret = append(ret, source)
		}
	}
	for _, item := range mapSliceFromValue(executionContext["missingDeliverableCoverage"]) {
		if source := failedCoverageSourceFromItem("deliverable", "deliverable", "missingDeliverableCoverage", item); source.TargetText != "" {
			ret = append(ret, source)
		}
	}
	return ret
}

func failedCoverageSourceFromItem(source, scope, pathPrefix string, item map[string]any) coverageWorkItemSource {
	text := firstNonEmptyString(
		stringFromMap(item, "targetText"),
		stringFromMap(item, "item"),
		stringFromMap(item, "goal"),
		stringFromMap(item, "goalItem"),
		stringFromMap(item, "requirement"),
		stringFromMap(item, "id"),
		stringFromMap(item, "deliverable"),
		stringFromMap(item, "name"),
		stringFromMap(item, "path"),
	)
	text = strings.TrimSpace(text)
	if text == "" || coverageTextLooksContainer(text) {
		return coverageWorkItemSource{}
	}
	id := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "goalRequirementId"), stringFromMap(item, "id"), stringFromMap(item, "coverageTargetToken")))
	return coverageWorkItemSource{
		Source:              source,
		SourceScope:         scope,
		SourceRequirementID: id,
		SourcePath:          pathPrefix + "." + coverageShortHash(item),
		TargetText:          text,
	}
}

func coverageWorkItemFromSource(order int, revision, key string, source coverageWorkItemSource, existing map[string]any, run *runModel.Run) map[string]any {
	status := "open"
	if existingStatus := strings.TrimSpace(stringFromMap(existing, "status")); existingStatus != "" {
		status = existingStatus
	}
	requiredKinds := coverageRequiredEvidenceKindsForSource(run, source)
	acceptedByKind := coverageEvidenceRefsByKind(existing, "acceptedEvidenceByKind", requiredKinds)
	rejectedByKind := coverageEvidenceRefsByKind(existing, "rejectedEvidenceByKind", requiredKinds)
	acceptedRefs := coverageFlattenEvidenceRefs(acceptedByKind, stringSliceFromValue(existing["acceptedEvidenceRefs"]))
	rejectedRefs := coverageFlattenEvidenceRefs(rejectedByKind, stringSliceFromValue(existing["rejectedEvidenceRefs"]))
	missingKinds := coverageMissingEvidenceKindsFromDetails(requiredKinds, acceptedByKind)
	if len(missingKinds) > 0 && (status == "satisfied" || status == "evidence_ready") {
		status = "open"
	}
	if len(missingKinds) == 0 && (status == "" || status == "open" || status == "in_progress") {
		status = "evidence_ready"
	}
	item := map[string]any{
		"schemaVersion":          coverageWorkItemSchema,
		"id":                     fmt.Sprintf("cwi-%03d", order),
		"source":                 source.Source,
		"sourceScope":            source.SourceScope,
		"definitionRevision":     revision,
		"sourceRequirementId":    source.SourceRequirementID,
		"sourcePath":             source.SourcePath,
		"targetText":             source.TargetText,
		"normalizedSourceKey":    key,
		"requiredEvidenceKinds":  requiredKinds,
		"status":                 status,
		"acceptedEvidenceRefs":   acceptedRefs,
		"rejectedEvidenceRefs":   rejectedRefs,
		"acceptedEvidenceByKind": acceptedByKind,
		"rejectedEvidenceByKind": rejectedByKind,
		"coverageBoundary":       "must stay within source targetText and profile-declared evidence requirements",
		"currentGap":             coverageCurrentGap(missingKinds),
		"reason":                 firstNonEmptyString(stringFromMap(existing, "reason"), "derived from current effective coverage baseline"),
	}
	if source.EvidenceRole != "" {
		item["evidenceRole"] = source.EvidenceRole
	}
	if source.SourceGoalID != "" {
		item["sourceGoalId"] = source.SourceGoalID
	}
	if source.SourceDoDID != "" {
		item["sourceDoDId"] = source.SourceDoDID
	}
	if source.ParentContext != "" {
		item["parentContext"] = source.ParentContext
	}
	if status == "satisfied" {
		item["currentGap"] = ""
	} else if strings.TrimSpace(stringFromMap(item, "currentGap")) == "" {
		item["currentGap"] = "required evidence kinds present; coverage assessment required"
	}
	return item
}

func coverageRequiredEvidenceKindsForSource(run *runModel.Run, source coverageWorkItemSource) []string {
	kinds := []string{"validation"}
	target := map[string]any{
		"kind":                source.SourceScope,
		"source":              source.Source,
		"id":                  source.SourceRequirementID,
		"targetText":          source.TargetText,
		"coverageTargetToken": source.SourceRequirementID,
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
	return normalizeEvidenceKindRequirementScope(source.SourceScope) == "deliverable"
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
		"id":                    stringFromMap(item, "id"),
		"source":                stringFromMap(item, "source"),
		"sourceScope":           stringFromMap(item, "sourceScope"),
		"evidenceRole":          stringFromMap(item, "evidenceRole"),
		"definitionRevision":    stringFromMap(item, "definitionRevision"),
		"sourceRequirementId":   stringFromMap(item, "sourceRequirementId"),
		"sourcePath":            stringFromMap(item, "sourcePath"),
		"targetText":            stringFromMap(item, "targetText"),
		"normalizedSourceKey":   stringFromMap(item, "normalizedSourceKey"),
		"requiredEvidenceKinds": stringSliceFromValue(item["requiredEvidenceKinds"]),
		"missingEvidenceKinds":  coverageMissingEvidenceKinds(item),
		"status":                firstNonEmptyString(stringFromMap(item, "status"), "open"),
		"currentGap":            stringFromMap(item, "currentGap"),
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

func coverageWorkItemLedgerSummary(item map[string]any, inFocus bool) map[string]any {
	ret := map[string]any{
		"id":                      stringFromMap(item, "id"),
		"sourceScope":             stringFromMap(item, "sourceScope"),
		"evidenceRole":            stringFromMap(item, "evidenceRole"),
		"sourceRequirementId":     stringFromMap(item, "sourceRequirementId"),
		"status":                  firstNonEmptyString(stringFromMap(item, "status"), "open"),
		"missingEvidenceKinds":    coverageMissingEvidenceKinds(item),
		"acceptedEvidenceRefs":    latestCoverageEvidenceRefs(stringSliceFromValue(item["acceptedEvidenceRefs"]), 2),
		"rejectedEvidenceRefs":    latestCoverageEvidenceRefs(stringSliceFromValue(item["rejectedEvidenceRefs"]), 2),
		"omittedFromCurrentFocus": !inFocus,
	}
	if !inFocus {
		ret["omittedReason"] = "context_budget_top_k"
	}
	return ret
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

func coverageHistoricalWorkItemSummary(item map[string]any, reason string) map[string]any {
	summary := coverageWorkItemLedgerSummary(item, false)
	summary["status"] = "superseded"
	summary["definitionRevision"] = stringFromMap(item, "definitionRevision")
	summary["historicalReason"] = reason
	return summary
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

func coverageWorkItemSourceKey(source coverageWorkItemSource) string {
	if strings.TrimSpace(source.Source) == "" || strings.TrimSpace(source.TargetText) == "" {
		return ""
	}
	id := strings.TrimSpace(source.SourceRequirementID)
	if id == "" {
		id = strings.TrimSpace(source.SourcePath)
	}
	if id == "" {
		id = coverageShortHash(source.TargetText)
	}
	return strings.Join([]string{source.Source, source.SourceScope, id}, ":")
}

func coverageRequirementPrefix(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return id + "-"
}

func coverageLeafTexts(text string) []map[string]string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	ret := make([]map[string]string, 0)
	parent := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			parent = strings.TrimSpace(strings.TrimLeft(line, "# "))
			continue
		}
		if match := coverageMarkdownLeafPattern.FindStringSubmatch(line); len(match) == 2 {
			leaf := strings.TrimSpace(match[1])
			if leaf != "" && !coverageTextLooksContainer(leaf) {
				ret = append(ret, map[string]string{"text": leaf, "parent": parent})
			}
			continue
		}
		if coverageTextLooksContainer(line) {
			parent = strings.TrimSuffix(line, ":")
			continue
		}
		if len(lines) == 1 {
			ret = append(ret, map[string]string{"text": line})
		}
	}
	if len(ret) == 0 && !coverageTextLooksContainer(text) {
		ret = append(ret, map[string]string{"text": text})
	}
	return ret
}

func coverageTextLooksContainer(text string) bool {
	text = strings.TrimSpace(text)
	return text == "" || strings.HasPrefix(text, "#") || strings.HasSuffix(text, ":") || strings.HasSuffix(text, "：")
}

func stableCoverageJSON(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(payload)
}

func coverageShortHash(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		payload = []byte(fmt.Sprintf("%v", value))
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])[:16]
}

func mapStringSliceValue(value any) map[string][]string {
	ret := map[string][]string{}
	switch typed := value.(type) {
	case map[string][]string:
		for key, values := range typed {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			ret[key] = compactUniqueStrings(values)
		}
	case map[string]any:
		for key, values := range typed {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			ret[key] = compactUniqueStrings(stringSliceFromValue(values))
		}
	}
	return ret
}
