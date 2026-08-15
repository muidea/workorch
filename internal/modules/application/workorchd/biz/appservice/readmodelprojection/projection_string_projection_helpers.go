package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation; source files are listed to preserve review traceability.

// source: execution_related_dod_items.go

func relatedDoDItemsForSection(node projectioncontract.TaskDecompositionNode, dodItems []string) []string {
	if len(dodItems) == 0 {
		return nil
	}
	haystack := strings.ToLower(strings.Join([]string{node.ID, node.Title, node.Kind}, " "))
	ret := make([]string, 0)
	for _, item := range dodItems {
		if strings.Contains(haystack, strings.ToLower(strings.TrimSpace(item))) {
			ret = append(ret, item)
		}
	}
	return ret
}

// source: llm_usage_breakdown.go

func llmUsageProviderModelBreakdownKey(item projectioncontract.LLMUsageTrace) string {
	return strings.TrimSpace(item.ProviderID) + "|" +
		strings.TrimSpace(item.Provider) + "|" +
		strings.TrimSpace(item.APISurface) + "|" +
		strings.TrimSpace(item.Model) + "|" +
		llmUsageProviderMode(item)
}

func newLLMUsageProviderModelBreakdown(item projectioncontract.LLMUsageTrace) *projectioncontract.LLMUsageProviderModelBreakdown {
	return &projectioncontract.LLMUsageProviderModelBreakdown{
		ProviderID:   strings.TrimSpace(item.ProviderID),
		Provider:     strings.TrimSpace(item.Provider),
		APISurface:   strings.TrimSpace(item.APISurface),
		Model:        strings.TrimSpace(item.Model),
		ProviderMode: llmUsageProviderMode(item),
	}
}

func addLLMUsageProviderModelBreakdown(entry *projectioncontract.LLMUsageProviderModelBreakdown, item projectioncontract.LLMUsageTrace, cacheReadTokens int, cacheWriteTokens int) {
	entry.EntryTotal++
	entry.InputTokens += item.InputTokens
	entry.OutputTokens += item.OutputTokens
	entry.TotalTokens += item.TotalTokens
	entry.CacheReadTokens += cacheReadTokens
	entry.CacheWriteTokens += cacheWriteTokens
	entry.CacheCreationTokens += llmUsagePayloadInt(item.CacheUsage, "cacheCreationTokens")
	if llmUsagePayloadInt(item.CacheMarkers, "renderedStablePrefixBytes") > 0 {
		entry.CacheablePromptCount++
	}
	if cacheReadTokens > 0 {
		entry.ActualCacheHitCount++
	}
	if cacheWriteTokens > 0 || llmUsagePayloadInt(item.CacheUsage, "cacheCreationTokens") > 0 {
		entry.CacheCreationCount++
	}
	if llmUsagePayloadBool(item.CacheMarkers, "promptCacheKeySent") {
		entry.PromptCacheKeySentCount++
	}
	if llmUsagePayloadBool(item.CacheMarkers, "cacheControlSent") {
		entry.CacheControlSentCount++
	}
	if llmUsagePayloadBool(item.CacheMarkers, "autoPrefixCacheExpected") {
		entry.AutoPrefixExpectedCount++
	}
	if item.Estimated {
		entry.EstimatedEntries++
	}
	if item.Status == "failed" {
		entry.FailedEntries++
	}
}

func llmUsageProviderMode(item projectioncontract.LLMUsageTrace) string {
	value, _ := item.CacheMarkers.String("providerMode")
	return strings.TrimSpace(value)
}

// source: progress_recovery_checkpoint.go

func applyRecoveryCheckpoint(recovery *projectioncontract.RecoveryPlan, latestCheckpoint *projectioncontract.RuntimeCheckpoint) {
	if recovery == nil || latestCheckpoint == nil {
		return
	}
	recovery.CheckpointID = latestCheckpoint.ID
	if recovery.BlockingTicketID == "" && recovery.BlockingApprovalID == "" && latestCheckpoint.CurrentStepID != "" {
		recovery.StepID = latestCheckpoint.CurrentStepID
	} else if recovery.StepID == "" {
		recovery.StepID = latestCheckpoint.CurrentStepID
	}
	if recovery.BlockingTicketID == "" {
		recovery.BlockingTicketID = latestCheckpoint.BlockingTicketID
	}
	if recovery.BlockingApprovalID == "" {
		recovery.BlockingApprovalID = latestCheckpoint.BlockingApprovalID
	}
	if recovery.FailureClass == "" {
		recovery.FailureClass = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureClass"]))
	}
	if recovery.FailureSignature == "" {
		recovery.FailureSignature = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureSignature"]))
	}
}

// source: repair_failure_candidates.go

func repairFailureProjectionFromCheckpoint(checkpoint projectioncontract.RuntimeCheckpoint) *repairFailureProjection {
	class := strings.TrimSpace(readModelStringValue(checkpoint.Data["failureClass"]))
	signature := strings.TrimSpace(readModelStringValue(checkpoint.Data["failureSignature"]))
	event := strings.TrimSpace(readModelStringValue(checkpoint.Data["event"]))
	if class == "" && signature == "" && event != "step_failed" {
		return nil
	}
	return &repairFailureProjection{
		stepID:      checkpoint.CurrentStepID,
		class:       class,
		signature:   signature,
		occurredAt:  checkpoint.OccurredAt,
		evidenceRef: checkpoint.ID,
	}
}

func repairFailureProjectionFromStep(step projectioncontract.StepView) *repairFailureProjection {
	class := strings.TrimSpace(readModelStringValue(step.Output["failureClass"]))
	signature := strings.TrimSpace(readModelStringValue(step.Output["failureSignature"]))
	if class == "" && signature == "" {
		return nil
	}
	return &repairFailureProjection{
		stepID:      step.ID,
		class:       class,
		signature:   signature,
		evidenceRef: step.ID,
	}
}

// source: repair_grounding_inputs.go

func applyRepairGroundingInputDetails(ret *projectioncontract.RepairLoop, repairStep *projectioncontract.StepView, actionPackage map[string]any) {
	if ret == nil || repairStep == nil {
		return
	}
	if compaction := readModelMapValue(repairStep.Input["repairInputCompaction"]); len(compaction) > 0 {
		ret.RepairInputCompaction = compaction
	}
	if repeated := readModelMapValue(actionPackage["repeatedTargetRepairMode"]); len(repeated) > 0 {
		ret.RepeatedFailureReason = strings.TrimSpace(readModelStringValue(repeated["reason"]))
	}
}

// source: repair_target_scope.go

func readModelFailureTargetScopeFromStep(step *projectioncontract.StepView) string {
	if step == nil {
		return ""
	}
	signature := readModelStringValue(step.Output["failureSignature"])
	if scope := readModelFailureTargetScope(signature); scope != "" {
		return scope
	}
	for _, scope := range []string{
		readModelStringValue(step.Output["failureTargetScope"]),
		readModelStringValue(step.Output["targetScope"]),
	} {
		if target := readModelFailureTargetScope(scope); target != "" {
			return target
		}
	}
	return strings.TrimSpace(readModelFirstNonEmptyString(
		readModelStringValue(step.Output["targetRef"]),
		readModelStringValue(step.Output["path"]),
	))
}

func readModelFailureTargetScope(signature string) string {
	for _, target := range readModelTargetsFromFailureSignature(signature) {
		return target
	}
	return ""
}

// source: result_quality_summary.go

type resultQualitySummaryFields struct {
	goalOutcome         string
	verificationSummary string
	knownLimitations    []string
	residualRisks       []string
	nextActions         []string
	evidenceRefs        []string
}

func resultQualitySummaryFieldsFor(item projectioncontract.Result) resultQualitySummaryFields {
	ret := resultQualitySummaryFields{}
	if item.SummaryDetail != nil {
		ret.goalOutcome = strings.TrimSpace(item.SummaryDetail.GoalOutcome)
		ret.verificationSummary = strings.TrimSpace(item.SummaryDetail.VerificationSummary)
		ret.knownLimitations = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.KnownLimitations))
		ret.residualRisks = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.ResidualRisks))
		ret.nextActions = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.NextActions))
		ret.evidenceRefs = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.EvidenceRefs))
	}
	if len(ret.evidenceRefs) == 0 {
		ret.evidenceRefs = compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs))
	}
	return ret
}

// source: result_quality_trace.go

func newResultQualityTrace(item projectioncontract.Result) projectioncontract.ResultQualityTrace {
	return projectioncontract.ResultQualityTrace{
		ResultID:     item.ID,
		DoDID:        item.DoDID,
		State:        "passed",
		Summary:      strings.TrimSpace(item.Summary),
		EvidenceRefs: compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs)),
	}
}

// source: step_view_lookup.go

func stepByID(steps []projectioncontract.StepView, stepID string) *projectioncontract.StepView {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return nil
	}
	for idx := range steps {
		if steps[idx].ID == stepID {
			return &steps[idx]
		}
	}
	return nil
}

// source: task_plan_projection_fields.go

type taskPlanFields struct {
	anchorStepID string
	stage        string
	state        string
	occurredAt   string
}

func taskPlanProjectionFields(decomposition projectioncontract.TaskDecomposition, step projectioncontract.StepView, hasStep bool, occurrences map[string]string) taskPlanFields {
	ret := taskPlanFields{
		anchorStepID: strings.TrimSpace(decomposition.RootStepID),
		state:        taskPlanStateFromDecomposition(decomposition.State),
		occurredAt:   strings.TrimSpace(occurrences[decomposition.RootStepID]),
	}
	if !hasStep {
		return ret
	}
	if root := strings.TrimSpace(readModelStringValue(step.Input["graphRootStepId"])); root != "" {
		ret.anchorStepID = root
	}
	ret.stage = strings.TrimSpace(readModelStringValue(step.Output["stage"]))
	if ret.stage == "" {
		ret.stage = strings.TrimSpace(readModelStringValue(step.Input["stage"]))
	}
	ret.state = taskPlanStateFromStep(step.State)
	return ret
}
