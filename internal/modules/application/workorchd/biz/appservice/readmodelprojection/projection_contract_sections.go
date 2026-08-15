package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation; source files are listed to preserve review traceability.

// source: capability_identity.go

func runtimeCapabilityIdentity(toolCall projectioncontract.ToolCall) projectioncontract.RuntimeCapability {
	source := toolCall.Output
	if len(source) == 0 {
		source = toolCall.Arguments
	}
	serverID := readModelStringValue(source["serverId"])
	operation := readModelStringValue(source["operation"])
	kind, name := runtimeCapabilityKindAndName(operation, source)
	if serverID == "" || kind == "" || name == "" {
		return projectioncontract.RuntimeCapability{}
	}
	id := defaultRuntimeCapabilityID(serverID, kind, name)
	return projectioncontract.RuntimeCapability{
		ID:          id,
		ServerID:    serverID,
		Kind:        kind,
		Name:        name,
		DisplayName: name,
	}
}

// source: compressed_context_refs.go

func compressedContextSectionRefsFromBundle(sections []projectioncontract.ContextSection) []string {
	sectionRefs := make([]string, 0, len(sections))
	for _, section := range sections {
		sectionRefs = append(sectionRefs, section.ID)
	}
	return compactUniqueStrings(sectionRefs)
}

func compressedContextSourceEvidenceRefs(progress projectioncontract.TaskProgress, recentEvidence []projectioncontract.ContextEvidence) []string {
	return compactUniqueStrings(append(progress.EvidenceRefs, contextEvidenceRefs(recentEvidence)...))
}

// source: context_recovery_section.go

func contextRecoverySection(recovery projectioncontract.RecoveryPlan) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:       "recovery",
		Kind:     "recovery",
		Title:    "Recovery recommendation",
		Priority: 100,
		State:    recovery.Mode,
		Summary:  recovery.NextAction,
		EvidenceRefs: compactUniqueStrings([]string{
			recovery.StepID,
			recovery.BlockingTicketID,
			recovery.BlockingApprovalID,
			recovery.CheckpointID,
		}),
		Data: map[string]any{
			"mode":               recovery.Mode,
			"nextAction":         recovery.NextAction,
			"runId":              recovery.RunID,
			"stepId":             recovery.StepID,
			"blockingTicketId":   recovery.BlockingTicketID,
			"blockingApprovalId": recovery.BlockingApprovalID,
			"checkpointId":       recovery.CheckpointID,
			"reason":             recovery.Reason,
		},
	}
}

// source: context_sections.go

func contextTaskProgressSection(progress projectioncontract.TaskProgress) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:           "task_progress",
		Kind:         "task_progress",
		Title:        "Task progress",
		Priority:     90,
		State:        progress.State,
		Summary:      progress.Summary,
		EvidenceRefs: progress.EvidenceRefs,
		Data: map[string]any{
			"currentMilestone": progress.CurrentMilestone,
			"milestones":       progress.Milestones,
			"blockers":         progress.Blockers,
			"recovery":         progress.Recovery,
		},
	}
}

// source: dod_evidence_trace.go

func dodEvidenceTraceFromResult(item projectioncontract.Result, evidenceIndex map[string]struct{}) projectioncontract.DoDEvidenceTrace {
	trace := projectioncontract.DoDEvidenceTrace{
		ResultID:  item.ID,
		DoDID:     item.DoDID,
		Satisfied: item.DoDAssessment.Satisfied,
		Summary:   item.DoDAssessment.Summary,
	}
	assessmentItems := item.DoDAssessment.Items
	if len(assessmentItems) == 0 && item.SummaryDetail != nil && len(item.SummaryDetail.DoDItems) > 0 {
		assessmentItems = item.SummaryDetail.DoDItems
	}
	if len(assessmentItems) > 0 {
		trace.Items = make([]projectioncontract.DoDEvidenceItem, 0, len(assessmentItems))
		for _, assessmentItem := range assessmentItems {
			trace.Items = append(trace.Items, dodEvidenceItemFromAssessment(assessmentItem, evidenceIndex))
		}
	}
	return trace
}

// source: llm_usage.go

func mapLLMUsageTracesFromEventRecords(records []projectioncontract.EventRecord, runID string) []projectioncontract.LLMUsageTrace {
	if len(records) == 0 {
		return nil
	}
	ret := make([]projectioncontract.LLMUsageTrace, 0)
	seen := make(map[string]struct{}, len(records))
	for _, item := range records {
		if !isLLMUsageEventRecordForRun(item, runID) {
			continue
		}
		trace, ok := llmUsageTraceFromEventRecord(item)
		if !ok {
			continue
		}
		// Eventstream delivery is at-least-once. A read projection must be
		// replay-safe and must not make one immutable audit event appear as two
		// provider submissions merely because the same EventRecord was read twice.
		identity := llmUsageTraceMergeKey(trace)
		if identity != "" {
			if _, duplicate := seen[identity]; duplicate {
				continue
			}
			seen[identity] = struct{}{}
		}
		ret = append(ret, trace)
	}
	if len(ret) == 0 {
		return nil
	}
	sortLLMUsageTraces(ret)
	return ret
}

// source: llm_usage_trace_projection.go

func llmUsageTraceFromEventRecord(item projectioncontract.EventRecord) (projectioncontract.LLMUsageTrace, bool) {
	if len(item.Payload.Fields) == 0 {
		return projectioncontract.LLMUsageTrace{}, false
	}
	estimated := llmUsageTraceEstimated(item.Payload)
	usageSource := llmUsageTraceUsageSource(item.Payload, estimated)
	return projectioncontract.LLMUsageTrace{
		ID:            item.ID,
		WorkspaceID:   llmUsageTraceWorkspaceID(item, item.Payload),
		RunID:         llmUsageTraceRunID(item, item.Payload),
		StepID:        llmUsageTraceStepID(item, item.Payload),
		ProviderID:    llmUsageTraceProviderID(item.Payload),
		Provider:      llmUsageTraceProvider(item.Payload),
		APISurface:    llmUsageTraceAPISurface(item.Payload),
		Model:         llmUsageTraceModel(item.Payload),
		Operation:     llmUsageTraceOperation(item.Payload),
		InputTokens:   llmUsageTraceInputTokens(item.Payload),
		OutputTokens:  llmUsageTraceOutputTokens(item.Payload),
		TotalTokens:   llmUsageTraceTotalTokens(item.Payload),
		Estimated:     estimated,
		UsageSource:   usageSource,
		RequestBytes:  llmUsageTraceRequestBytes(item.Payload),
		ResponseBytes: llmUsageTraceResponseBytes(item.Payload),
		Streaming:     llmUsageTraceStreaming(item.Payload),
		ContextBudget: llmUsageTraceContextBudget(item.Payload),
		CacheUsage:    llmUsageTraceCacheUsage(item.Payload),
		CacheMarkers:  llmUsageTraceCacheMarkers(item.Payload),
		StartedAt:     llmUsageTraceStartedAt(item.Payload),
		CompletedAt:   llmUsageTraceCompletedAt(item.Payload),
		DurationMs:    llmUsageTraceDurationMs(item.Payload),
		Status:        llmUsageTraceStatus(item.Payload),
		FailureClass:  llmUsageTraceFailureClass(item.Payload),
		OccurredAt:    item.OccurredAt,
		EvidenceRef:   llmUsageTraceEvidenceRef(item),
	}, true
}

// source: progress_evidence_artifact_ids.go

func idsFromResults(items []projectioncontract.Result) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
		ret = append(ret, item.ArtifactRefs...)
	}
	return compactUniqueStrings(ret)
}

func idsFromArtifacts(items []projectioncontract.Artifact) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
	}
	return compactUniqueStrings(ret)
}

// source: progress_evidence_ids.go

func idsFromSteps(items []projectioncontract.StepView) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromToolCalls(items []projectioncontract.ToolCall) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromCheckpoints(items []projectioncontract.RuntimeCheckpoint) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

// source: progress_governance_items.go

func decisionProgressItem(item projectioncontract.Decision) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "decision",
		Title:        firstNonEmptyTimelineString(item.Question, item.DecisionKind, item.ID),
		State:        item.State,
		EvidenceRefs: []string{item.ID, item.StepID},
	}
}

func approvalProgressItem(item projectioncontract.Approval) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "approval",
		Title:        firstNonEmptyTimelineString(item.Reason, item.ID),
		State:        item.State,
		EvidenceRefs: []string{item.ID, item.StepID},
	}
}

// source: progress_milestone_state.go

func currentProgressMilestone(items []projectioncontract.ProgressMilestone) string {
	for _, item := range items {
		if item.State != "completed" {
			return item.ID
		}
	}
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1].ID
}

func blockerReason(items []projectioncontract.ProgressBlocker, kind string, id string) string {
	for _, item := range items {
		if item.Kind == kind && item.ID == id {
			return item.Reason
		}
	}
	return ""
}

// source: progress_output_items.go

func resultProgressItem(item projectioncontract.Result) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "result",
		Title:        firstNonEmptyTimelineString(item.Title, item.SourceType, item.ID),
		State:        item.State,
		EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.SourceRef}, item.EvidenceRefs...)),
	}
}

func artifactProgressItem(item projectioncontract.Artifact) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "artifact",
		Title:        firstNonEmptyTimelineString(item.Name, item.URI, item.ArtifactType, item.ID),
		State:        item.State,
		EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.ResultID, item.StepID, item.ToolCallID, item.PatchID, item.ValidationResultID}, item.EvidenceRefs...)),
	}
}

// source: progress_patch_validation_items.go

func patchProgressItem(item projectioncontract.Patch) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "patch",
		Title:        firstNonEmptyTimelineString(item.TargetRef, item.PatchType, item.ID),
		State:        item.State,
		EvidenceRefs: []string{item.ID, item.ToolCallID, item.StepID},
	}
}

func validationProgressItem(item projectioncontract.ValidationResult, recoveredHistoricalFailures bool) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "validation_result",
		Title:        firstNonEmptyTimelineString(item.ValidationKind, item.TargetRef, item.ID),
		State:        recoveredProgressItemState(item.State, recoveredHistoricalFailures),
		EvidenceRefs: []string{item.ID, item.EvidenceRef, item.StepID},
	}
}

// source: progress_recovery_blockers.go

func applyBlockingRecovery(recovery *projectioncontract.RecoveryPlan, blockers []projectioncontract.ProgressBlocker) bool {
	if recovery == nil {
		return false
	}
	if recovery.BlockingTicketID != "" {
		recovery.Mode = "wait_decision"
		recovery.NextAction = "resolve_decision_ticket"
		recovery.Reason = blockerReason(blockers, "decision", recovery.BlockingTicketID)
		return true
	}
	if recovery.BlockingApprovalID != "" {
		recovery.Mode = "wait_approval"
		recovery.NextAction = "resolve_approval"
		recovery.Reason = blockerReason(blockers, "approval", recovery.BlockingApprovalID)
		return true
	}
	return false
}

// source: progress_runtime_items.go

func stepProgressItem(item projectioncontract.StepView, recoveredHistoricalFailures bool) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "step",
		Title:        firstNonEmptyTimelineString(item.Kind, item.ID),
		State:        recoveredProgressItemState(item.State, recoveredHistoricalFailures),
		EvidenceRefs: []string{item.ID},
	}
}

// source: progress_tool_call_item.go

func toolCallProgressItem(item projectioncontract.ToolCall, recoveredHistoricalFailures bool) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "tool_call",
		Title:        firstNonEmptyTimelineString(item.ToolName, item.ID),
		State:        recoveredProgressItemState(item.State, recoveredHistoricalFailures),
		EvidenceRefs: []string{item.ID, item.StepID},
	}
}

// source: repair_state.go

func applyRepairLoopRecoveryState(ret *projectioncontract.RepairLoop, recovery projectioncontract.RecoveryPlan, attemptCount int, hasLastFailure bool) {
	if ret == nil {
		return
	}
	switch recovery.Mode {
	case "wait_decision":
		ret.State = "awaiting_decision"
	case "wait_approval":
		ret.State = "awaiting_approval"
	case "none":
		if attemptCount > 0 {
			ret.State = "completed_after_repair"
		}
	case "inspect_failure", "restart":
		ret.State = "terminated"
	default:
		if attemptCount > 0 || hasLastFailure {
			ret.State = "repairing"
		}
	}
}

// source: task_decomposition_node_accumulator.go

func addTaskDecompositionNode(item *projectioncontract.TaskDecomposition, node map[string]any, executedByNode map[string]projectioncontract.StepView, rootMeta taskLifecycleMetadata, blockingTickets map[string]string) {
	taskNode := taskDecompositionNode(node, executedByNode)
	meta := rootMeta
	if taskNode.StepID != "" && executedByNode != nil {
		if step, ok := executedByNode[taskNode.ID]; ok {
			meta = taskLifecycleMetadataFromStep(step)
			if ticketID := blockingTickets[step.ID]; ticketID != "" {
				meta.blockingTicketID = ticketID
			}
		}
	}
	applyTaskLifecycleToNode(&taskNode, taskLifecycleMetadataForNode(taskNode, meta))
	item.Nodes = append(item.Nodes, taskNode)
	item.EvidenceRefs = append(item.EvidenceRefs, taskNode.EvidenceRefs...)
	switch taskNode.State {
	case "completed":
		item.NodeCompleted++
	case "failed":
		item.NodeFailed++
	case "running":
		item.NodeRunning++
	}
}

// source: timeline_memory_recall_item.go

func memoryRecallTimelineItem(trace projectioncontract.MemoryRecallTrace, runState string) projectioncontract.RuntimeTimelineItem {
	return projectioncontract.RuntimeTimelineItem{
		Source:     "memory_recall",
		Type:       trace.Source,
		ID:         trace.ID,
		RunState:   runState,
		StepID:     trace.StepID,
		Summary:    memoryRecallTimelineSummary(trace),
		OccurredAt: trace.OccurredAt,
		Data: map[string]any{
			"stage":              trace.Stage,
			"source":             trace.Source,
			"policy":             trace.Policy,
			"status":             trace.Status,
			"retrievedMemoryIds": append([]string(nil), trace.RetrievedMemoryIDs...),
			"reason":             trace.Reason,
		},
	}
}

// source: timeline_runtime_items.go

func stepRuntimeTimelineItem(step projectioncontract.StepView, runState string, occurredAt string) projectioncontract.RuntimeTimelineItem {
	return projectioncontract.RuntimeTimelineItem{
		Source:     "step",
		Type:       step.Kind,
		ID:         step.ID,
		RunState:   runState,
		StepID:     step.ID,
		StepState:  step.State,
		Summary:    timelineStepSummary(step.Output),
		OccurredAt: occurredAt,
		Output:     boundedStreamStepOutput(step.Output),
	}
}

func checkpointRuntimeTimelineItem(checkpoint projectioncontract.RuntimeCheckpoint) projectioncontract.RuntimeTimelineItem {
	return projectioncontract.RuntimeTimelineItem{
		Source:     "checkpoint",
		Type:       checkpoint.CheckpointType,
		ID:         checkpoint.ID,
		RunState:   checkpoint.RunStateSnapshot,
		StepID:     checkpoint.CurrentStepID,
		OccurredAt: checkpoint.OccurredAt,
		Data:       cloneReadModelMap(checkpoint.Data),
	}
}

// source: workspace_result_artifact_merge.go

func mergeWorkspaceSnapshotResults(existing []projectioncontract.Result, latest []projectioncontract.Result) []projectioncontract.Result {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.Result, 0, len(existing)+len(latest))
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

func mergeWorkspaceSnapshotArtifacts(existing []projectioncontract.Artifact, latest []projectioncontract.Artifact) []projectioncontract.Artifact {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.Artifact, 0, len(existing)+len(latest))
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
