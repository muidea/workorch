package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

// WD5 mechanical consolidation; source files are listed to preserve review traceability.

// source: approval_view_projection.go

func projectApprovalView(item projectioncontract.Approval) projectioncontract.Approval {
	return projectioncontract.Approval{
		ID:             item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		Action:         item.Action,
		State:          item.State,
		AutoGovernance: item.AutoGovernance,
		ResolvedAt:     item.ResolvedAt,
		ResolvedBy:     item.ResolvedBy,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

// source: artifact_view_projection.go

func projectArtifactView(item projectioncontract.Artifact) projectioncontract.Artifact {
	return projectioncontract.Artifact{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		ResultID:           item.ResultID,
		StepID:             item.StepID,
		ToolCallID:         item.ToolCallID,
		PatchID:            item.PatchID,
		ValidationResultID: item.ValidationResultID,
		ArtifactType:       item.ArtifactType,
		Category:           item.Category,
		Name:               item.Name,
		Summary:            item.Summary,
		URI:                item.URI,
		ContentType:        item.ContentType,
		State:              item.State,
		EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

// source: capability_context_mapping.go

func runtimeCapabilityFromContext(raw map[string]any) projectioncontract.RuntimeCapability {
	id := readModelStringValue(raw["id"])
	return projectioncontract.RuntimeCapability{
		ID:             id,
		ServerID:       readModelStringValue(raw["serverId"]),
		Kind:           readModelStringValue(raw["kind"]),
		Name:           readModelStringValue(raw["name"]),
		DisplayName:    firstNonEmptyTimelineString(readModelStringValue(raw["displayName"]), readModelStringValue(raw["name"]), id),
		Description:    readModelStringValue(raw["description"]),
		Category:       readModelStringValue(raw["category"]),
		Tags:           readModelStringSliceValue(raw["tags"]),
		RiskLevel:      readModelStringValue(raw["riskLevel"]),
		Enabled:        readModelBoolValue(raw["enabled"]),
		ApprovalPolicy: readModelStringValue(raw["approvalPolicy"]),
		Status:         readModelStringValue(raw["status"]),
	}
}

// source: checkpoint_selection.go

func latestRuntimeCheckpoint(items []projectioncontract.RuntimeCheckpoint) *projectioncontract.RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	latest := items[0]
	for _, item := range items[1:] {
		if item.OccurredAt > latest.OccurredAt ||
			(item.OccurredAt == latest.OccurredAt && item.ID > latest.ID) {
			latest = item
		}
	}
	return &latest
}

// source: checkpoint_view_projection.go

func projectRuntimeCheckpointView(item projectioncontract.RuntimeCheckpoint) projectioncontract.RuntimeCheckpoint {
	return projectioncontract.RuntimeCheckpoint{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		CheckpointType:     item.CheckpointType,
		CurrentStepID:      item.CurrentStepID,
		RunStateSnapshot:   item.RunStateSnapshot,
		BlockingTicketID:   item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID,
		ThreadRef:          item.ThreadRef,
		OccurredAt:         item.OccurredAt,
		RestorableUntil:    item.RestorableUntil,
	}
}

// source: context_compression_candidates.go

func contextCompressionCandidates(bundle projectioncontract.ContextBundle) []projectioncontract.ContextCompressionCandidate {
	var ret []projectioncontract.ContextCompressionCandidate
	if len(bundle.RecentEvidence) > 8 {
		ret = append(ret, projectioncontract.ContextCompressionCandidate{
			SectionID:     "recent_evidence",
			Reason:        "recent evidence exceeds compact recovery window",
			Retention:     "retain latest evidence ids, states, summaries, and timestamps",
			SourceRefs:    contextEvidenceRefs(bundle.RecentEvidence),
			SuggestedMode: "summarize",
		})
	}
	if len(bundle.TaskProgress.Items) > 10 {
		ret = append(ret, projectioncontract.ContextCompressionCandidate{
			SectionID:     "task_progress",
			Reason:        "task progress contains many low-level items",
			Retention:     "retain milestones, blockers, recovery, and accepted result refs",
			SourceRefs:    bundle.TaskProgress.EvidenceRefs,
			SuggestedMode: "milestone_summary",
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: context_evidence_data.go

func compactContextEvidenceData(item projectioncontract.RuntimeTimelineItem) map[string]any {
	data := map[string]any{}
	if item.Topic != "" {
		data["topic"] = item.Topic
	}
	if item.RunState != "" {
		data["runState"] = item.RunState
	}
	if item.StepState != "" {
		data["stepState"] = item.StepState
	}
	if item.Summary != "" {
		data["summary"] = item.Summary
	}
	if len(data) == 0 {
		return nil
	}
	return data
}

// source: context_evidence_refs.go

func lastContextEvidence(items []projectioncontract.ContextEvidence, limit int) []projectioncontract.ContextEvidence {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	return append([]projectioncontract.ContextEvidence(nil), items[start:]...)
}

func contextEvidenceRefs(items []projectioncontract.ContextEvidence) []string {
	var refs []string
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return compactUniqueStrings(refs)
}

// source: context_execution_section_state.go

func contextExecutionSectionStateAndEvidenceRefs(items []projectioncontract.ExecutionSection) (string, []string) {
	refs := make([]string, 0)
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.ActualEvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return state, refs
}

// source: context_recent_evidence.go

func contextRecentEvidence(timeline []projectioncontract.RuntimeTimelineItem, limit int) []projectioncontract.ContextEvidence {
	if len(timeline) == 0 || limit <= 0 {
		return nil
	}
	start := len(timeline) - limit
	if start < 0 {
		start = 0
	}
	ret := make([]projectioncontract.ContextEvidence, 0, len(timeline)-start)
	for _, item := range timeline[start:] {
		ret = append(ret, contextEvidenceFromTimelineItem(item))
	}
	return ret
}

func contextEvidenceFromTimelineItem(item projectioncontract.RuntimeTimelineItem) projectioncontract.ContextEvidence {
	return projectioncontract.ContextEvidence{
		ID:         firstNonEmptyTimelineString(item.ID, item.Topic, item.StepID),
		Kind:       firstNonEmptyTimelineString(item.Source+"/"+item.Type, item.Source),
		State:      firstNonEmptyTimelineString(item.StepState, item.RunState),
		Summary:    item.Summary,
		OccurredAt: item.OccurredAt,
		EvidenceRefs: compactUniqueStrings([]string{
			item.ID,
			item.StepID,
			item.Topic,
		}),
		Data: compactContextEvidenceData(item),
	}
}

// source: context_recent_evidence_section.go

func contextRecentEvidenceSection(items []projectioncontract.ContextEvidence) projectioncontract.ContextSection {
	if len(items) == 0 {
		return projectioncontract.ContextSection{}
	}
	refs := make([]string, 0, len(items))
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return projectioncontract.ContextSection{
		ID:           "recent_evidence",
		Kind:         "recent_evidence",
		Title:        "Recent evidence",
		Priority:     70,
		Summary:      "latest runtime timeline items retained for recovery",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"items": items,
		},
	}
}

// source: context_result_refs.go

func resultRefsFromTaskProgress(progress projectioncontract.TaskProgress) []string {
	var refs []string
	for _, item := range progress.Items {
		if item.Kind == "result" || item.Kind == "artifact" {
			refs = append(refs, item.ID)
			refs = append(refs, item.EvidenceRefs...)
		}
	}
	return compactUniqueStrings(refs)
}

// source: decision_view_projection.go

func projectDecisionView(item projectioncontract.Decision) projectioncontract.Decision {
	return projectioncontract.Decision{
		ID:                      item.ID,
		WorkspaceID:             item.WorkspaceID,
		RunID:                   item.RunID,
		StepID:                  item.StepID,
		Reason:                  item.Reason,
		DecisionKind:            item.DecisionKind,
		Question:                item.Question,
		Context:                 projectDecisionContextView(item.Context),
		FailureClass:            item.FailureClass,
		FailureSignature:        item.FailureSignature,
		AllowedRecoveryActions:  append([]string(nil), item.AllowedRecoveryActions...),
		DefinitionClarification: projectioncontract.CloneDefinitionClarification(item.DefinitionClarification),
		State:                   item.State,
		ResolutionID:            item.ResolutionID,
		ResolutionType:          item.ResolutionType,
		ResolutionPayload:       projectDecisionPayloadView(item.ResolutionPayload),
		AutoGovernance:          item.AutoGovernance,
		ResolvedAt:              item.ResolvedAt,
		ResolvedBy:              item.ResolvedBy,
		CreatedAt:               item.CreatedAt,
		UpdatedAt:               item.UpdatedAt,
	}
}

// source: execution_section_summary.go

func executionSectionSummary(node projectioncontract.TaskDecompositionNode) string {
	switch node.State {
	case "completed":
		return "section completed with linked runtime evidence"
	case "failed":
		return "section failed; inspect linked evidence and governance records"
	case "running":
		return "section is running"
	default:
		return "section is planned"
	}
}

// source: llm_context_budget_summary.go

const llmContextBudgetRecentWindowSize = 10

func summarizeLLMContextBudgets(items []projectioncontract.LLMContextBudgetTrace) projectioncontract.LLMContextBudgetSummary {
	ret := projectioncontract.LLMContextBudgetSummary{}
	if len(items) == 0 {
		return ret
	}
	var utilizationTotal float64
	var utilizationCount int
	for _, item := range items {
		ret.TraceTotal++
		if item.Truncated {
			ret.TruncatedCount++
		}
		if item.OverBudget {
			ret.OverBudgetCount++
		}
		if item.P0Overflow {
			ret.P0OverflowCount++
		}
		if item.GoalDoDCompressed {
			ret.GoalDoDCompressedCount++
		}
		if item.PromptCaptured {
			ret.PromptCapturedCount++
		}
		if item.ShrinkLevel > ret.MaxShrinkLevel {
			ret.MaxShrinkLevel = item.ShrinkLevel
		}
		if item.EstimatedInputTokens > ret.MaxInputTokens {
			ret.MaxInputTokens = item.EstimatedInputTokens
		}
		if item.ActualTokens > ret.MaxInputTokens {
			ret.MaxInputTokens = item.ActualTokens
		}
		if item.MaxRequestBytes > ret.MaxRequestBytes {
			ret.MaxRequestBytes = item.MaxRequestBytes
		}
		if item.ContextUtilization > 0 {
			utilizationTotal += item.ContextUtilization
			utilizationCount++
			if item.ContextUtilization > ret.MaxContextUtilization {
				ret.MaxContextUtilization = item.ContextUtilization
			}
		}
	}
	if utilizationCount > 0 {
		ret.AvgContextUtilization = utilizationTotal / float64(utilizationCount)
	}
	applyLLMContextBudgetRecentWindowSummary(&ret, items)
	return ret
}

func applyLLMContextBudgetRecentWindowSummary(summary *projectioncontract.LLMContextBudgetSummary, items []projectioncontract.LLMContextBudgetTrace) {
	if summary == nil || len(items) == 0 {
		return
	}
	windowSize := llmContextBudgetRecentWindowSize
	if len(items) < windowSize {
		windowSize = len(items)
	}
	recentStart := len(items) - windowSize
	recent := summarizeLLMContextBudgetWindow(items[recentStart:])
	summary.RecentWindowSize = windowSize
	summary.RecentTraceTotal = recent.traceTotal
	summary.RecentTruncatedCount = recent.truncatedCount
	summary.RecentOverBudgetCount = recent.overBudgetCount
	summary.RecentMaxUtilization = recent.maxUtilization
	summary.RecentAvgUtilization = recent.avgUtilization
	summary.RecentMaxInputTokens = recent.maxInputTokens
	previousStart := recentStart - windowSize
	if previousStart < 0 {
		previousStart = 0
	}
	if previousStart < recentStart {
		previous := summarizeLLMContextBudgetWindow(items[previousStart:recentStart])
		summary.UtilizationTrend = llmContextBudgetUtilizationTrend(previous.avgUtilization, recent.avgUtilization)
	}
}

type llmContextBudgetWindowSummary struct {
	traceTotal      int
	truncatedCount  int
	overBudgetCount int
	maxUtilization  float64
	avgUtilization  float64
	maxInputTokens  int
}

func summarizeLLMContextBudgetWindow(items []projectioncontract.LLMContextBudgetTrace) llmContextBudgetWindowSummary {
	ret := llmContextBudgetWindowSummary{}
	var utilizationTotal float64
	var utilizationCount int
	for _, item := range items {
		ret.traceTotal++
		if item.Truncated {
			ret.truncatedCount++
		}
		if item.OverBudget {
			ret.overBudgetCount++
		}
		if item.EstimatedInputTokens > ret.maxInputTokens {
			ret.maxInputTokens = item.EstimatedInputTokens
		}
		if item.ActualTokens > ret.maxInputTokens {
			ret.maxInputTokens = item.ActualTokens
		}
		if item.ContextUtilization > 0 {
			utilizationTotal += item.ContextUtilization
			utilizationCount++
			if item.ContextUtilization > ret.maxUtilization {
				ret.maxUtilization = item.ContextUtilization
			}
		}
	}
	if utilizationCount > 0 {
		ret.avgUtilization = utilizationTotal / float64(utilizationCount)
	}
	return ret
}

func llmContextBudgetUtilizationTrend(previous float64, recent float64) string {
	if previous <= 0 || recent <= 0 {
		return ""
	}
	delta := recent - previous
	switch {
	case delta > 0.05:
		return "increasing"
	case delta < -0.05:
		return "decreasing"
	default:
		return "stable"
	}
}

// source: llm_usage_cache_summary.go

func addLLMUsageCacheTokenMetrics(summary *projectioncontract.LLMUsageSummary, metrics llmUsageCacheMetrics) {
	summary.CacheReadTokens += metrics.cacheReadTokens
	summary.CacheWriteTokens += metrics.cacheWriteTokens
	summary.CacheCreationTokens += metrics.cacheCreationTokens
	summary.CacheableInputTokens += metrics.cacheableInputTokens
	summary.NonCacheableInputTokens += metrics.nonCacheableInputTokens
}

func addLLMUsageCacheEntryMetrics(summary *projectioncontract.LLMUsageSummary, metrics llmUsageCacheMetrics) {
	if metrics.cacheEstimatedEntry {
		summary.CacheEstimatedEntries++
	}
	if metrics.cacheProviderNativeEntry {
		summary.CacheProviderEntries++
	}
}

// source: llm_usage_event_filter.go

func isLLMUsageEventRecordForRun(item projectioncontract.EventRecord, runID string) bool {
	if item.Topic != "/run/llm_usage/recorded" {
		return false
	}
	return runID == "" || item.RunID == runID
}

// source: llm_usage_ratios.go

func applyLLMUsageSummaryRatios(summary *projectioncontract.LLMUsageSummary) {
	summary.CacheReadInputRatio = readModelRatio(summary.CacheReadTokens, summary.InputTokens)
	summary.CacheableInputRatio = readModelRatio(summary.CacheableInputTokens, summary.InputTokens)
	summary.NonCacheableInputRatio = readModelRatio(summary.NonCacheableInputTokens, summary.InputTokens)
	for idx := range summary.ProviderModelBreakdown {
		summary.ProviderModelBreakdown[idx].CacheReadInputRatio = readModelRatio(summary.ProviderModelBreakdown[idx].CacheReadTokens, summary.ProviderModelBreakdown[idx].InputTokens)
	}
}

func readModelRatio(part, total int) float64 {
	if total <= 0 || part <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

// source: llm_usage_summary.go

func summarizeLLMUsage(items []projectioncontract.LLMUsageTrace) projectioncontract.LLMUsageSummary {
	ret := projectioncontract.LLMUsageSummary{}
	breakdownByKey := map[string]*projectioncontract.LLMUsageProviderModelBreakdown{}
	for _, item := range items {
		addLLMUsageTraceSummary(&ret, breakdownByKey, item)
	}
	ret.ProviderModelBreakdown = sortedLLMUsageProviderModelBreakdowns(breakdownByKey)
	applyLLMUsageSummaryRatios(&ret)
	return ret
}

// source: llm_usage_trace_merge_key.go

func llmUsageTraceMergeKey(item projectioncontract.LLMUsageTrace) string {
	return firstNonEmptyTimelineString(item.ID, item.StepID+"|"+item.OccurredAt)
}

// source: patch_view_projection.go

func projectPatchView(item projectioncontract.Patch) projectioncontract.Patch {
	return projectioncontract.Patch{
		ID:               item.ID,
		WorkspaceID:      item.WorkspaceID,
		RunID:            item.RunID,
		StepID:           item.StepID,
		ToolCallID:       item.ToolCallID,
		ApprovalID:       item.ApprovalID,
		RevertApprovalID: item.RevertApprovalID,
		PatchType:        item.PatchType,
		TargetRef:        item.TargetRef,
		Summary:          item.Summary,
		State:            item.State,
		AddedLines:       item.AddedLines,
		DeletedLines:     item.DeletedLines,
		BeforeHash:       item.BeforeHash,
		AfterHash:        item.AfterHash,
		AffectedFiles:    cloneReadModelStringSlice(item.AffectedFiles),
		RiskLevel:        item.RiskLevel,
		RiskReasons:      cloneReadModelStringSlice(item.RiskReasons),
		Diagnostics:      cloneReadModelMapSlice(item.Diagnostics),
		CreatedAt:        item.CreatedAt,
		AppliedAt:        item.AppliedAt,
		RevertedAt:       item.RevertedAt,
	}
}

// source: progress_patch_validation_evidence_ids.go

func idsFromPatches(items []projectioncontract.Patch) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromValidations(items []projectioncontract.ValidationResult) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID, item.EvidenceRef)
	}
	return compactUniqueStrings(ret)
}

// source: repair_attempt_count.go

func repairAttemptProjection(steps []projectioncontract.StepView) (count int, lastAttempt int, lastStepID string) {
	for _, step := range steps {
		if !readModelBoolValue(step.Input["runtimeRepairFollowUp"]) {
			continue
		}
		count++
		attempt := readModelIntValue(step.Input["runtimeRepairAttempt"])
		if attempt <= 0 {
			attempt = 1
		}
		if attempt > lastAttempt || (attempt == lastAttempt && step.ID > lastStepID) {
			lastAttempt = attempt
			lastStepID = step.ID
		}
	}
	if count > 0 && lastAttempt == 0 {
		lastAttempt = count
	}
	return count, lastAttempt, lastStepID
}

// source: repair_structured_signals.go

func readModelStructuredFailureSignalsFromStep(step *projectioncontract.StepView, failureClass string, failureSignature string) map[string]any {
	if step == nil {
		return nil
	}
	if signals := readModelMapValue(step.Output["structuredFailureSignals"]); len(signals) > 0 {
		return signals
	}
	return readModelFallbackStructuredFailureSignals(step.Output, failureClass, failureSignature)
}

// source: result_quality_summary_issues.go

func checkResultQualitySummaryFields(quality *projectioncontract.ResultQualityTrace, summary resultQualitySummaryFields) {
	if summary.goalOutcome == "" {
		appendResultQualityIssue(quality, "missing_goal_outcome", "error", "result is missing a concrete goal outcome summary", "populate summaryDetail.goalOutcome with the delivered outcome", summary.evidenceRefs...)
	}
	if summary.verificationSummary == "" {
		appendResultQualityIssue(quality, "missing_verification_summary", "error", "result is missing verification summary", "populate summaryDetail.verificationSummary with verification scope and conclusions", summary.evidenceRefs...)
	}
	if len(summary.evidenceRefs) == 0 {
		appendResultQualityIssue(quality, "missing_evidence_refs", "error", "result is missing evidence references", "attach validation, patch, tool call or artifact evidence refs")
	}
	if len(summary.knownLimitations) == 0 && len(summary.residualRisks) == 0 {
		appendResultQualityIssue(quality, "missing_risk_disclosure", "error", "result is missing known limitations and residual risks", "populate summaryDetail.knownLimitations or summaryDetail.residualRisks", summary.evidenceRefs...)
	}
	if len(summary.nextActions) == 0 {
		appendResultQualityIssue(quality, "missing_next_actions", "warning", "result is missing next actions", "populate summaryDetail.nextActions for follow-up guidance", summary.evidenceRefs...)
	}
}

// source: snapshot_step_references.go

func referencedSnapshotStepIDs(steps []projectioncontract.StepView) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
		if previous := previousStepIDSnapshot(step); previous != "" {
			ret[previous] = true
		}
	}
	return ret
}

// source: snapshot_terminal_step_candidates.go

func terminalSnapshotStepCandidates(steps []projectioncontract.StepView) []*projectioncontract.StepView {
	referenced := referencedSnapshotStepIDs(steps)
	var terminals []*projectioncontract.StepView
	for idx := range steps {
		if steps[idx].ID == "" || referenced[steps[idx].ID] {
			continue
		}
		terminals = append(terminals, &steps[idx])
	}
	return terminals
}

// source: step_stream_timeline.go

func streamTimelineItems(step projectioncontract.StepView, runState string, stepOccurredAt string) []projectioncontract.RuntimeTimelineItem {
	if len(step.Output) == 0 {
		return nil
	}
	raw, ok := step.Output["streamEvents"]
	if !ok {
		return streamDiagnosticsTimelineItems(step, runState, stepOccurredAt)
	}
	events := readModelMapSliceValue(raw)
	if len(events) == 0 {
		return streamDiagnosticsTimelineItems(step, runState, stepOccurredAt)
	}
	ret := make([]projectioncontract.RuntimeTimelineItem, 0, streamTimelineItemCapacity(len(events)))
	omittedInserted := false
	for idx, event := range events {
		if streamTimelineEventOmitted(idx, len(events)) {
			if !omittedInserted {
				ret = append(ret, streamTimelineOmittedItem(step, runState, len(events)))
				omittedInserted = true
			}
			continue
		}
		ret = append(ret, streamTimelineEventItem(step, runState, stepOccurredAt, idx, event))
	}
	return ret
}

// source: task_execution_steps.go

func taskExecutionStepsByRoot(steps []projectioncontract.StepView) map[string]map[string]projectioncontract.StepView {
	ret := map[string]map[string]projectioncontract.StepView{}
	for _, step := range steps {
		nodeID := readModelStringValue(step.Input["graphNodeId"])
		if nodeID == "" {
			continue
		}
		rootStepID := readModelStringValue(step.Input["graphRootStepId"])
		if rootStepID == "" {
			rootStepID = graphRootStepIDFromStepID(step.ID)
		}
		if rootStepID == "" {
			continue
		}
		if ret[rootStepID] == nil {
			ret[rootStepID] = map[string]projectioncontract.StepView{}
		}
		ret[rootStepID][nodeID] = step
	}
	return ret
}

// source: task_node_clone.go

func cloneTaskDecompositionNodes(items []projectioncontract.TaskDecompositionNode) []projectioncontract.TaskDecompositionNode {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.TaskDecompositionNode, 0, len(items))
	for _, item := range items {
		clone := item
		clone.DependsOn = cloneReadModelStringSlice(item.DependsOn)
		clone.EvidenceRefs = cloneReadModelStringSlice(item.EvidenceRefs)
		clone.CoverageTargets = cloneReadModelStringSlice(item.CoverageTargets)
		clone.Input = cloneReadModelMap(item.Input)
		ret = append(ret, clone)
	}
	return ret
}

// source: timeline_event_filter.go

func timelineEvents(items []projectioncontract.RuntimeTimelineItem) []projectioncontract.RuntimeTimelineItem {
	ret := make([]projectioncontract.RuntimeTimelineItem, 0, len(items))
	for _, item := range items {
		if item.Source == "event" || item.Source == "persisted_event" {
			ret = append(ret, item)
		}
	}
	return ret
}

// source: timeline_step_checkpoint_source.go

func isStepRuntimeCheckpoint(checkpoint projectioncontract.RuntimeCheckpoint) bool {
	if checkpoint.CurrentStepID == "" || len(checkpoint.Data) == 0 {
		return false
	}
	if readModelStringValue(checkpoint.Data["stepState"]) != "" {
		return true
	}
	eventName := readModelStringValue(checkpoint.Data["event"])
	return eventName == "step_dispatched" || eventName == "step_completed" || eventName == "step_failed"
}

// source: timeline_step_observed_source.go

func timelineStepIDFromObservedEvent(item projectioncontract.ObservedView) (string, bool) {
	if item.Topic != "/step/dispatched" && item.Topic != "/step/state/changed" && item.SubjectType != "step" {
		return "", false
	}
	stepID := item.StepID
	if stepID == "" && item.SubjectType == "step" {
		stepID = item.SubjectID
	}
	if stepID == "" {
		return "", false
	}
	return stepID, true
}

// source: timeline_step_occurrence_sources.go

func timelineStepIDFromEventRecord(item projectioncontract.EventRecord) (string, bool) {
	if item.Topic != "/step/dispatched" && item.Topic != "/step/state/changed" && item.SubjectType != "step" {
		return "", false
	}
	stepID := item.StepID
	if stepID == "" && item.SubjectType == "step" {
		stepID = item.SubjectID
	}
	if stepID == "" {
		return "", false
	}
	return stepID, true
}

// source: used_capability_index.go

func runtimeCapabilityIndex(items []projectioncontract.RuntimeCapability) map[string]projectioncontract.RuntimeCapability {
	ret := map[string]projectioncontract.RuntimeCapability{}
	for _, item := range items {
		ret[item.ID] = item
	}
	return ret
}

func mergeUsedRuntimeCapability(current projectioncontract.RuntimeCapability, identity projectioncontract.RuntimeCapability, catalog map[string]projectioncontract.RuntimeCapability, selected map[string]projectioncontract.RuntimeCapability, toolCallID string, stepID string) projectioncontract.RuntimeCapability {
	item := current
	if item.ID == "" {
		if loaded, exists := catalog[identity.ID]; exists {
			item = loaded
		} else if selectedItem, exists := selected[identity.ID]; exists {
			item = selectedItem
		} else {
			item = identity
		}
	}
	item.UseCount++
	item.EvidenceRefs = compactUniqueStrings(append(item.EvidenceRefs, toolCallID, stepID))
	return item
}

// source: validation_view_projection.go

func projectValidationResultView(item projectioncontract.ValidationResult) projectioncontract.ValidationResult {
	return projectioncontract.ValidationResult{
		ID:              item.ID,
		WorkspaceID:     item.WorkspaceID,
		RunID:           item.RunID,
		StepID:          item.StepID,
		ValidationKind:  item.ValidationKind,
		TargetRef:       item.TargetRef,
		State:           item.State,
		Summary:         item.Summary,
		EvidenceRef:     item.EvidenceRef,
		EvidenceQuality: item.EvidenceQuality,
		EvidenceReason:  item.EvidenceReason,
		ZeroWork:        item.ZeroWork,
		EvidenceTags:    cloneReadModelStringSlice(item.EvidenceTags),
		StartedAt:       item.StartedAt,
		EndedAt:         item.EndedAt,
	}
}

// source: workspace_artifact_continuation_merge.go

func mergeWorkspaceSnapshotArtifactContinuations(existing []projectioncontract.ArtifactContinuation, latest []projectioncontract.ArtifactContinuation) []projectioncontract.ArtifactContinuation {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.ArtifactContinuation, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

// source: workspace_llm_usage_merge.go

func mergeWorkspaceLLMUsageTraces(existing []projectioncontract.LLMUsageTrace, latest []projectioncontract.LLMUsageTrace) []projectioncontract.LLMUsageTrace {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.LLMUsageTrace, 0, len(existing)+len(latest))
	for _, item := range existing {
		key := llmUsageTraceMergeKey(item)
		seen[key] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		key := llmUsageTraceMergeKey(item)
		if idx, ok := seen[key]; ok {
			out[idx] = item
			continue
		}
		seen[key] = len(out)
		out = append(out, item)
	}
	sortLLMUsageTraces(out)
	return out
}

// source: workspace_merge.go

func mergeWorkspaceSnapshotPatches(existing []projectioncontract.Patch, latest []projectioncontract.Patch) []projectioncontract.Patch {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.Patch, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

// source: workspace_outcome_apply.go

func applyWorkspaceRunOutcome(ret *projectioncontract.WorkspaceDisplayOutcome, outcome projectioncontract.RunDisplayOutcome) {
	if ret == nil {
		return
	}
	ret.State = outcome.State
	ret.RunID = outcome.RunID
	ret.ResultID = outcome.ResultID
	ret.Accepted = outcome.Accepted
	ret.Terminal = outcome.Terminal
	ret.Summary = outcome.Summary
	ret.Reason = outcome.Reason
	ret.EvidenceRefs = outcome.EvidenceRefs
	ret.ArtifactRefs = outcome.ArtifactRefs
}
