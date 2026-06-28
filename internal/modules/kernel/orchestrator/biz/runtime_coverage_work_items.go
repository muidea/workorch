package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	coverageWorkItemsKey          = "coverageWorkItems"
	coverageWorkItemContextKey    = "coverageWorkItemContext"
	coverageWorkItemSchema        = "workorch.coverage_work_item.v1"
	coverageWorkItemContextSchema = "workorch.coverage_work_item_context.v1"
)

type coverageWorkItemSource struct {
	Source                 string
	SourceScope            string
	SourceGoalID           string
	SourceDoDID            string
	SourceRequirementID    string
	SourceAliases          []string
	SourcePath             string
	TargetText             string
	EvidenceRole           string
	RequiredEvidenceKinds  []string
	RequiredEvidenceCounts map[string]int
	ParentContext          string
	ExcludedReason         string
}

type coverageWorkItemUpdate struct {
	WorkItemIDs       []string
	TargetTokens      []string
	EvidenceKinds     []string
	EvidenceRefs      []string
	Accepted          bool
	CloseSatisfied    bool
	Reason            string
	AcceptanceOptions coverageEvidenceAcceptanceOptions
}

type coverageSubsystem struct {
	items repository.CoverageWorkItemRepository
}

func newCoverageSubsystem(items ...repository.CoverageWorkItemRepository) *coverageSubsystem {
	subsystem := &coverageSubsystem{}
	if len(items) > 0 {
		subsystem.items = items[0]
	}
	return subsystem
}

func (c *coverageSubsystem) ensureFromRepository(ctx context.Context, run *runModel.Run) (bool, error) {
	if c == nil || c.items == nil || run == nil || run.ID == "" {
		return c.ensure(run, ctx), nil
	}
	hydrated, err := c.hydrateProjectionFromRepository(coverageRepositoryContext(run, ctx), run)
	if err != nil {
		return hydrated, err
	}
	ensured := c.ensure(run, ctx)
	return hydrated || ensured, nil
}

func (c *coverageSubsystem) ensure(run *runModel.Run, contexts ...context.Context) bool {
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
			c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, nil)
			return true
		}
		if _, ok := run.ExecutionContext[coverageWorkItemContextKey]; ok {
			delete(run.ExecutionContext, coverageWorkItemContextKey)
			c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, nil)
			return true
		}
		c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, nil)
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
		c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, items)
		return false
	}
	run.ExecutionContext[coverageWorkItemsKey] = items
	run.ExecutionContext[coverageWorkItemContextKey] = context
	synchronizeAcceptanceProjection(run.ExecutionContext, items, context)
	c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, items)
	return true
}

func (c *coverageSubsystem) hydrateProjectionFromRepository(ctx context.Context, run *runModel.Run) (bool, error) {
	if c == nil || c.items == nil || run == nil || run.ID == "" {
		return false, nil
	}
	items, err := c.items.ListByRun(ctx, run.ID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	projection := coverageWorkItemProjectionFromModels(items)
	if len(projection) == 0 {
		return false, nil
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	current := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if stableCoverageJSON(current) == stableCoverageJSON(projection) {
		return false, nil
	}
	run.ExecutionContext[coverageWorkItemsKey] = projection
	return true, nil
}

func (c *coverageSubsystem) prepareReplanSuccessor(run *runModel.Run, contexts ...context.Context) bool {
	if run == nil {
		return false
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	return c.ensure(run, contexts...)
}

func (c *coverageSubsystem) updateFromCompletedStep(run *runModel.Run, step *runModel.Step, contexts ...context.Context) bool {
	if run == nil || step == nil || step.State != common.StepStateCompleted {
		return false
	}
	c.ensure(run, contexts...)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return false
	}
	update := coverageWorkItemStepUpdate(step)
	if len(update.EvidenceKinds) == 0 || (len(update.WorkItemIDs) == 0 && len(update.TargetTokens) == 0) {
		changed := updateConvergenceBudgetFromStep(run, step, false)
		synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
		return changed
	}
	changed := false
	for _, item := range items {
		if !coverageWorkItemMatchesUpdate(item, update) {
			continue
		}
		if coverageApplyWorkItemEvidenceWithOptions(item, update.EvidenceKinds, update.EvidenceRefs, true, false, string(step.ID), "", update.AcceptanceOptions) {
			changed = true
		}
	}
	budgetChanged := updateConvergenceBudgetFromStep(run, step, changed)
	if changed {
		revision := coverageDefinitionRevision(run.ExecutionContext)
		run.ExecutionContext[coverageWorkItemsKey] = items
		run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(revision, items, coverageExcludedSourcesFromContext(run.ExecutionContext), coverageHistoricalItemsFromContext(run.ExecutionContext))
	}
	synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
	c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, items)
	return changed || budgetChanged
}

func (c *coverageSubsystem) updateFromFailedStep(run *runModel.Run, step *runModel.Step, reason string, contexts ...context.Context) bool {
	if run == nil || step == nil || step.State != common.StepStateFailed {
		return false
	}
	c.ensure(run, contexts...)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return false
	}
	update := coverageWorkItemStepUpdate(step)
	update.Accepted = false
	update.Reason = firstNonEmptyString(strings.TrimSpace(reason), stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason"))
	if len(update.EvidenceKinds) == 0 || (len(update.WorkItemIDs) == 0 && len(update.TargetTokens) == 0) {
		changed := updateConvergenceBudgetFromStep(run, step, false)
		synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
		return changed
	}
	changed := false
	for _, item := range items {
		if !coverageWorkItemMatchesUpdate(item, update) {
			continue
		}
		if coverageApplyWorkItemEvidenceWithOptions(item, update.EvidenceKinds, update.EvidenceRefs, false, false, string(step.ID), update.Reason, update.AcceptanceOptions) {
			changed = true
		}
	}
	budgetChanged := updateConvergenceBudgetFromStep(run, step, changed)
	if changed {
		revision := coverageDefinitionRevision(run.ExecutionContext)
		run.ExecutionContext[coverageWorkItemsKey] = items
		run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(revision, items, coverageExcludedSourcesFromContext(run.ExecutionContext), coverageHistoricalItemsFromContext(run.ExecutionContext))
	}
	synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
	c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, items)
	return changed || budgetChanged
}

func (c *coverageSubsystem) updateFromAssessment(run *runModel.Run, step *runModel.Step, assessment map[string]any, contexts ...context.Context) bool {
	if run == nil || step == nil || len(assessment) == 0 {
		return false
	}
	c.ensure(run, contexts...)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return false
	}
	updates := coverageWorkItemAssessmentUpdates(step, assessment)
	if len(updates) == 0 {
		changed := updateConvergenceBudgetFromStep(run, step, false)
		synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
		return changed
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
			if coverageApplyWorkItemEvidenceWithOptions(item, update.EvidenceKinds, update.EvidenceRefs, update.Accepted, update.CloseSatisfied, string(step.ID), update.Reason, update.AcceptanceOptions) {
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
	budgetChanged := updateConvergenceBudgetFromStep(run, step, changed)
	if changed {
		revision := coverageDefinitionRevision(run.ExecutionContext)
		run.ExecutionContext[coverageWorkItemsKey] = items
		run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(revision, items, coverageExcludedSourcesFromContext(run.ExecutionContext), coverageHistoricalItemsFromContext(run.ExecutionContext))
	}
	synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
	c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, items)
	return changed || budgetChanged
}

func (c *coverageSubsystem) updateFromGovernanceEvidence(run *runModel.Run, decisionRefs []string, approvalRefs []string, updatedBy string, contexts ...context.Context) bool {
	if run == nil {
		return false
	}
	if len(decisionRefs) == 0 && len(approvalRefs) == 0 {
		return false
	}
	c.ensure(run, contexts...)
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
	if changed {
		revision := coverageDefinitionRevision(run.ExecutionContext)
		run.ExecutionContext[coverageWorkItemsKey] = items
		run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(revision, items, coverageExcludedSourcesFromContext(run.ExecutionContext), coverageHistoricalItemsFromContext(run.ExecutionContext))
		synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
	}
	c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, items)
	return changed
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

func (c *coverageSubsystem) validateReadyForAcceptance(run *runModel.Run, contexts ...context.Context) error {
	if run == nil {
		return nil
	}
	hydrated := false
	if c != nil && c.items != nil && run.ID != "" {
		var err error
		hydrated, err = c.hydrateProjectionFromRepository(coverageRepositoryContext(run, contexts...), run)
		if err != nil {
			return err
		}
	}
	if !hydrated {
		c.ensure(run, contexts...)
	}
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
		if coverageWorkItemCompletionReady(item) {
			continue
		}
		state := deriveCoverageWorkItemState(item)
		missing := state.MissingEvidenceKinds
		if len(missing) == 0 && state.NeedsAssessment() {
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

func (c *coverageSubsystem) syncRepositoryProjection(ctx context.Context, run *runModel.Run, items []map[string]any) bool {
	if c == nil || c.items == nil || run == nil || run.ID == "" {
		return false
	}
	existing, err := c.items.ListByRun(ctx, run.ID)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return false
	}
	existingByID := map[common.CoverageWorkItemID]*runtimecontract.CoverageWorkItem{}
	for _, item := range existing {
		if item == nil || item.ID == "" {
			continue
		}
		existingByID[item.ID] = item
	}
	seen := map[common.CoverageWorkItemID]bool{}
	changed := false
	for _, item := range items {
		model := coverageWorkItemModelFromProjection(run, item)
		if model == nil {
			continue
		}
		seen[model.ID] = true
		existingItem := existingByID[model.ID]
		if existingItem == nil {
			if err := c.items.Create(ctx, model); err == nil {
				changed = true
			} else if errors.Is(err, common.ErrConflict) {
				if updateErr := c.items.Update(ctx, model); updateErr == nil {
					changed = true
				}
			}
			continue
		}
		if stableCoverageJSON(existingItem) == stableCoverageJSON(model) {
			continue
		}
		if err := c.items.Update(ctx, model); err == nil {
			changed = true
		}
	}
	for _, existingItem := range existing {
		if existingItem == nil || existingItem.ID == "" || seen[existingItem.ID] || !existingItem.IsActive() {
			continue
		}
		superseded := *existingItem
		superseded.Status = "superseded"
		superseded.CurrentGap = ""
		if superseded.Metadata == nil {
			superseded.Metadata = map[string]any{}
		}
		superseded.Metadata["supersededByCoverageProjectionSync"] = true
		if err := c.items.Update(ctx, &superseded); err == nil {
			changed = true
		}
	}
	return changed
}

func coverageRepositoryContext(run *runModel.Run, contexts ...context.Context) context.Context {
	var ctx context.Context
	for _, candidate := range contexts {
		if candidate != nil {
			ctx = candidate
			break
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if run == nil || run.WorkspaceID == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
}

func coverageWorkItemModelFromProjection(run *runModel.Run, item map[string]any) *runtimecontract.CoverageWorkItem {
	if run == nil || run.ID == "" || len(item) == 0 {
		return nil
	}
	contextID := strings.TrimSpace(stringFromMap(item, "id"))
	if contextID == "" {
		return nil
	}
	metadata := cloneStepPayload(item)
	metadata["contextCoverageWorkItemId"] = contextID
	model := &runtimecontract.CoverageWorkItem{
		ID:                    coverageWorkItemRepositoryID(run.ID, contextID),
		WorkspaceID:           run.WorkspaceID,
		RunID:                 run.ID,
		DefinitionRevision:    strings.TrimSpace(stringFromMap(item, "definitionRevision")),
		NormalizedSourceKey:   strings.TrimSpace(stringFromMap(item, "normalizedSourceKey")),
		SourceScope:           strings.TrimSpace(stringFromMap(item, "sourceScope")),
		SourceRequirementID:   strings.TrimSpace(stringFromMap(item, "sourceRequirementId")),
		SourceGoalID:          common.GoalID(strings.TrimSpace(stringFromMap(item, "sourceGoalId"))),
		SourceDoDID:           common.DoDID(strings.TrimSpace(stringFromMap(item, "sourceDoDId"))),
		SourcePath:            strings.TrimSpace(stringFromMap(item, "sourcePath")),
		TargetText:            strings.TrimSpace(stringFromMap(item, "targetText")),
		RequiredEvidenceKinds: compactUniqueStrings(stringSliceFromValue(item["requiredEvidenceKinds"])),
		Status:                firstNonEmptyString(strings.TrimSpace(stringFromMap(item, "status")), "open"),
		AcceptedEvidenceRefs:  compactUniqueStrings(stringSliceFromValue(item["acceptedEvidenceRefs"])),
		RejectedEvidenceRefs:  compactUniqueStrings(stringSliceFromValue(item["rejectedEvidenceRefs"])),
		CurrentGap:            strings.TrimSpace(stringFromMap(item, "currentGap")),
		UpdatedByStepID:       common.StepID(strings.TrimSpace(stringFromMap(item, "updatedByStepId"))),
		Metadata:              metadata,
	}
	if model.DefinitionRevision == "" || model.NormalizedSourceKey == "" {
		return nil
	}
	return model
}

func coverageWorkItemRepositoryID(runID common.RunID, contextID string) common.CoverageWorkItemID {
	contextID = strings.TrimSpace(contextID)
	if runID == "" || contextID == "" {
		return ""
	}
	return common.CoverageWorkItemID(string(runID) + "-" + contextID)
}

func coverageWorkItemProjectionFromModels(items []*runtimecontract.CoverageWorkItem) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	sorted := append([]*runtimecontract.CoverageWorkItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := sorted[i]
		right := sorted[j]
		if left == nil || right == nil {
			return right != nil
		}
		leftContextID := coverageWorkItemContextIDFromModel(left)
		rightContextID := coverageWorkItemContextIDFromModel(right)
		if leftContextID != rightContextID {
			return leftContextID < rightContextID
		}
		if left.DefinitionRevision != right.DefinitionRevision {
			return left.DefinitionRevision < right.DefinitionRevision
		}
		if left.NormalizedSourceKey != right.NormalizedSourceKey {
			return left.NormalizedSourceKey < right.NormalizedSourceKey
		}
		return left.ID < right.ID
	})
	ret := make([]map[string]any, 0, len(sorted))
	for _, item := range sorted {
		mapped := coverageWorkItemProjectionFromModel(item)
		if len(mapped) > 0 {
			ret = append(ret, mapped)
		}
	}
	return ret
}

func coverageWorkItemProjectionFromModel(item *runtimecontract.CoverageWorkItem) map[string]any {
	if item == nil || item.ID == "" {
		return nil
	}
	ret := cloneStepPayload(item.Metadata)
	if ret == nil {
		ret = map[string]any{}
	}
	contextID := coverageWorkItemContextIDFromModel(item)
	if contextID != "" {
		ret["id"] = contextID
	} else if strings.TrimSpace(stringFromMap(ret, "id")) == "" {
		ret["id"] = string(item.ID)
	}
	ret["repositoryId"] = string(item.ID)
	ret["schemaVersion"] = firstNonEmptyString(stringFromMap(ret, "schemaVersion"), coverageWorkItemSchema)
	if item.WorkspaceID != "" {
		ret["workspaceId"] = string(item.WorkspaceID)
	}
	if item.RunID != "" {
		ret["runId"] = string(item.RunID)
	}
	if item.DefinitionRevision != "" {
		ret["definitionRevision"] = item.DefinitionRevision
	}
	if item.NormalizedSourceKey != "" {
		ret["normalizedSourceKey"] = item.NormalizedSourceKey
	}
	if item.SourceScope != "" {
		ret["sourceScope"] = item.SourceScope
	}
	if item.SourceRequirementID != "" {
		ret["sourceRequirementId"] = item.SourceRequirementID
	}
	if item.SourceGoalID != "" {
		ret["sourceGoalId"] = string(item.SourceGoalID)
	}
	if item.SourceDoDID != "" {
		ret["sourceDoDId"] = string(item.SourceDoDID)
	}
	if item.SourcePath != "" {
		ret["sourcePath"] = item.SourcePath
	}
	if item.TargetText != "" {
		ret["targetText"] = item.TargetText
	}
	if len(item.RequiredEvidenceKinds) > 0 {
		ret["requiredEvidenceKinds"] = append([]string(nil), item.RequiredEvidenceKinds...)
	}
	if item.Status != "" {
		ret["status"] = item.Status
	}
	if len(item.AcceptedEvidenceRefs) > 0 {
		ret["acceptedEvidenceRefs"] = append([]string(nil), item.AcceptedEvidenceRefs...)
	}
	if len(item.RejectedEvidenceRefs) > 0 {
		ret["rejectedEvidenceRefs"] = append([]string(nil), item.RejectedEvidenceRefs...)
	}
	if item.CurrentGap != "" {
		ret["currentGap"] = item.CurrentGap
	}
	if item.UpdatedByStepID != "" {
		ret["updatedByStepId"] = string(item.UpdatedByStepID)
	}
	return ret
}

func coverageWorkItemContextIDFromModel(item *runtimecontract.CoverageWorkItem) string {
	if item == nil {
		return ""
	}
	if id := strings.TrimSpace(stringFromMap(item.Metadata, "contextCoverageWorkItemId")); id != "" {
		return id
	}
	if id := strings.TrimSpace(stringFromMap(item.Metadata, "id")); id != "" {
		return id
	}
	return ""
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
