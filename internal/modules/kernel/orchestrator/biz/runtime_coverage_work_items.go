package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

type coverageWorkItemSource struct {
	Source              string
	SourceScope         string
	SourceGoalID        string
	SourceDoDID         string
	SourceRequirementID string
	SourceAliases       []string
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

type coverageSubsystem struct{}

func newCoverageSubsystem() *coverageSubsystem {
	return &coverageSubsystem{}
}

func (c *coverageSubsystem) ensure(run *runModel.Run) bool {
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
		acceptanceChanged := synchronizeAcceptanceProjection(run.ExecutionContext, nil, nil)
		if _, ok := run.ExecutionContext[coverageWorkItemsKey]; ok {
			delete(run.ExecutionContext, coverageWorkItemsKey)
			delete(run.ExecutionContext, coverageWorkItemContextKey)
			return true
		}
		if _, ok := run.ExecutionContext[coverageWorkItemContextKey]; ok {
			delete(run.ExecutionContext, coverageWorkItemContextKey)
			return true
		}
		return acceptanceChanged
	}
	acceptanceContext := cloneStepPayload(run.ExecutionContext)
	if acceptanceContext == nil {
		acceptanceContext = map[string]any{}
	}
	acceptanceChanged := synchronizeAcceptanceProjection(acceptanceContext, items, context)
	if stableCoverageJSON(currentItems) == stableCoverageJSON(items) &&
		stableCoverageJSON(currentContext) == stableCoverageJSON(context) &&
		!acceptanceChanged {
		return false
	}
	run.ExecutionContext[coverageWorkItemsKey] = items
	run.ExecutionContext[coverageWorkItemContextKey] = context
	synchronizeAcceptanceProjection(run.ExecutionContext, items, context)
	return true
}

func (c *coverageSubsystem) prepareReplanSuccessor(run *runModel.Run) bool {
	if run == nil {
		return false
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	return c.ensure(run)
}

func (c *coverageSubsystem) updateFromCompletedStep(run *runModel.Run, step *runModel.Step) bool {
	if run == nil || step == nil || step.State != common.StepStateCompleted {
		return false
	}
	c.ensure(run)
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
	synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
	return true
}

func (c *coverageSubsystem) updateFromFailedStep(run *runModel.Run, step *runModel.Step, reason string) bool {
	if run == nil || step == nil || step.State != common.StepStateFailed {
		return false
	}
	c.ensure(run)
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
	synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
	return true
}

func (c *coverageSubsystem) updateFromAssessment(run *runModel.Run, step *runModel.Step, assessment map[string]any) bool {
	if run == nil || step == nil || len(assessment) == 0 {
		return false
	}
	c.ensure(run)
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
	synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
	return true
}

func (c *coverageSubsystem) updateFromGovernanceEvidence(run *runModel.Run, decisionRefs []string, approvalRefs []string, updatedBy string) bool {
	if run == nil {
		return false
	}
	c.ensure(run)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return false
	}
	changed := false
	for _, item := range items {
		required := stringSet(stringSliceFromValue(item["requiredEvidenceKinds"]))
		if required["decision"] && len(decisionRefs) > 0 {
			if coverageApplyWorkItemEvidence(item, []string{"decision"}, decisionRefs, true, false, updatedBy, "synchronized from resolved Decision owner evidence") {
				changed = true
			}
		}
		if required["approval"] && len(approvalRefs) > 0 {
			if coverageApplyWorkItemEvidence(item, []string{"approval"}, approvalRefs, true, false, updatedBy, "synchronized from approved Approval owner evidence") {
				changed = true
			}
		}
	}
	if !changed {
		return false
	}
	revision := coverageDefinitionRevision(run.ExecutionContext)
	run.ExecutionContext[coverageWorkItemsKey] = items
	run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(revision, items, coverageExcludedSourcesFromContext(run.ExecutionContext), coverageHistoricalItemsFromContext(run.ExecutionContext))
	synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
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

func (c *coverageSubsystem) validateReadyForAcceptance(run *runModel.Run) error {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	c.ensure(run)
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
	existingByTextKey := map[string]map[string]any{}
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
		if textKey := coverageWorkItemTextKey(
			stringFromMap(item, "source"),
			stringFromMap(item, "sourceScope"),
			stringFromMap(item, "targetText"),
		); textKey != "" {
			existingByTextKey[textKey] = item
		}
	}
	items := make([]map[string]any, 0, len(sources))
	seen := map[string]bool{}
	matchedExistingKeys := map[string]bool{}
	matchedExistingTextKeys := map[string]bool{}
	for _, source := range sources {
		key := coverageWorkItemSourceKey(source)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		existingKey := key
		existing := existingByKey[key]
		if len(existing) == 0 {
			for _, aliasKey := range coverageWorkItemSourceAliasKeys(source) {
				if candidate := existingByKey[aliasKey]; len(candidate) > 0 {
					existing = candidate
					existingKey = aliasKey
					break
				}
			}
		}
		if len(existing) == 0 {
			if textKey := coverageWorkItemTextKey(source.Source, source.SourceScope, source.TargetText); textKey != "" {
				if candidate := existingByTextKey[textKey]; len(candidate) > 0 {
					existing = candidate
					matchedExistingTextKeys[textKey] = true
				}
			}
		}
		if existingKey != "" && len(existing) > 0 {
			matchedExistingKeys[existingKey] = true
		}
		item := coverageWorkItemFromSource(len(items)+1, revision, key, source, existing, run)
		items = append(items, item)
	}
	supersededKeys := make([]string, 0, len(existingByKey))
	for key := range existingByKey {
		if !seen[key] && !matchedExistingKeys[key] {
			item := existingByKey[key]
			textKey := coverageWorkItemTextKey(stringFromMap(item, "source"), stringFromMap(item, "sourceScope"), stringFromMap(item, "targetText"))
			if textKey == "" || !matchedExistingTextKeys[textKey] {
				supersededKeys = append(supersededKeys, key)
			}
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

func coverageWorkItemFromSource(order int, revision, key string, source coverageWorkItemSource, existing map[string]any, run *runModel.Run) map[string]any {
	status := "open"
	if existingStatus := strings.TrimSpace(stringFromMap(existing, "status")); existingStatus != "" {
		status = existingStatus
	}
	requiredKinds := coverageRequiredEvidenceKindsForSource(run, source)
	requiredCounts := coverageRequiredEvidenceCountsForSource(source, requiredKinds)
	acceptedByKind := coverageEvidenceRefsByKind(existing, "acceptedEvidenceByKind", requiredKinds)
	rejectedByKind := coverageEvidenceRefsByKind(existing, "rejectedEvidenceByKind", requiredKinds)
	acceptedRefs := coverageFlattenEvidenceRefs(acceptedByKind, stringSliceFromValue(existing["acceptedEvidenceRefs"]))
	rejectedRefs := coverageFlattenEvidenceRefs(rejectedByKind, stringSliceFromValue(existing["rejectedEvidenceRefs"]))
	missingKinds := coverageMissingEvidenceKindsFromDetails(requiredKinds, acceptedByKind, requiredCounts)
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
		"requiredEvidenceCounts": requiredCounts,
		"status":                 status,
		"acceptedEvidenceRefs":   acceptedRefs,
		"rejectedEvidenceRefs":   rejectedRefs,
		"acceptedEvidenceByKind": acceptedByKind,
		"rejectedEvidenceByKind": rejectedByKind,
		"missingEvidenceKinds":   missingKinds,
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
	if len(source.SourceAliases) > 0 {
		item["sourceRequirementAliases"] = source.SourceAliases
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

func coverageHistoricalWorkItemSummary(item map[string]any, reason string) map[string]any {
	summary := coverageWorkItemLedgerSummary(item, false)
	summary["status"] = "superseded"
	summary["definitionRevision"] = stringFromMap(item, "definitionRevision")
	summary["historicalReason"] = reason
	return summary
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

func mapIntValue(value any) map[string]int {
	ret := map[string]int{}
	switch typed := value.(type) {
	case map[string]int:
		for key, count := range typed {
			key = strings.TrimSpace(key)
			if key == "" || count <= 0 {
				continue
			}
			ret[key] = count
		}
	case map[string]any:
		for key, value := range typed {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			count := int(int64FromValue(value))
			if count <= 0 {
				continue
			}
			ret[key] = count
		}
	}
	return ret
}
