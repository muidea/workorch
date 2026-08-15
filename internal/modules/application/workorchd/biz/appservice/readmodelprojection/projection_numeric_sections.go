package readmodelprojection

import (
	"strconv"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation; source files are listed to preserve review traceability.

// source: context_bundle_summaries.go

func contextDoDSummary(dod map[string]any) string {
	if len(dod) == 0 {
		return ""
	}
	switch items := dod["items"].(type) {
	case []string:
		return strconv.Itoa(len(items)) + " DoD items"
	case []any:
		return strconv.Itoa(len(items)) + " DoD items"
	default:
		return ""
	}
}

func formalOutputSummary(items []projectioncontract.Result) string {
	if result := primaryTaskProgressResult(items); result != nil {
		return result.Summary
	}
	return ""
}

// source: context_execution_sections.go

func contextExecutionSectionsSection(items []projectioncontract.ExecutionSection) projectioncontract.ContextSection {
	if len(items) == 0 {
		return projectioncontract.ContextSection{}
	}
	state, refs := contextExecutionSectionStateAndEvidenceRefs(items)
	return projectioncontract.ContextSection{
		ID:           "execution_sections",
		Kind:         "execution_sections",
		Title:        "Execution sections",
		Priority:     84,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " execution sections",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"executionSections": items,
		},
	}
}

// source: context_memory_sections.go

func contextMemoryRecallSection(items []projectioncontract.MemoryRecallTrace) projectioncontract.ContextSection {
	if len(items) == 0 {
		return projectioncontract.ContextSection{}
	}
	refs := make([]string, 0)
	memoryIDs := make([]string, 0)
	status := "selected"
	for _, item := range items {
		refs = append(refs, item.ID, item.EvidenceRef)
		memoryIDs = append(memoryIDs, item.RetrievedMemoryIDs...)
		if item.Status == "degraded" {
			status = "degraded"
		}
	}
	return projectioncontract.ContextSection{
		ID:           "memory_recall",
		Kind:         "memory_recall",
		Title:        "Memory recall",
		Priority:     83,
		State:        status,
		Summary:      strconv.Itoa(len(items)) + " memory recall traces",
		EvidenceRefs: compactUniqueStrings(append(refs, memoryIDs...)),
		Data: map[string]any{
			"memoryRecallTraces": items,
			"memoryIds":          compactUniqueStrings(memoryIDs),
		},
	}
}

// source: context_task_sections.go

func contextTaskDecompositionSection(items []projectioncontract.TaskDecomposition) projectioncontract.ContextSection {
	if len(items) == 0 {
		return projectioncontract.ContextSection{}
	}
	refs := make([]string, 0, len(items))
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return projectioncontract.ContextSection{
		ID:           "task_decomposition",
		Kind:         "task_decomposition",
		Title:        "Task decomposition",
		Priority:     85,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " decomposition graphs",
		EvidenceRefs: refs,
		Data: map[string]any{
			"taskDecompositions": items,
		},
	}
}

// source: step_stream_diagnostics.go

func streamDiagnosticsTimelineItems(step projectioncontract.StepView, runState string, stepOccurredAt string) []projectioncontract.RuntimeTimelineItem {
	diagnostics := readModelMapValue(step.Output["streamDiagnostics"])
	if len(diagnostics) == 0 {
		return nil
	}
	summary := "stream diagnostics"
	if receivedBytes := readModelIntValue(diagnostics["receivedBytes"]); receivedBytes > 0 {
		summary = "received " + strconv.Itoa(receivedBytes) + " stream bytes"
	}
	return []projectioncontract.RuntimeTimelineItem{{
		Source:     "step_stream_summary",
		Type:       "summary",
		ID:         step.ID + "/stream/summary",
		RunState:   runState,
		StepID:     step.ID,
		StepState:  step.State,
		Summary:    summary,
		OccurredAt: stepOccurredAt,
		Data:       cloneReadModelMap(diagnostics),
	}}
}

// source: step_stream_omitted_item.go

func streamTimelineOmittedItem(step projectioncontract.StepView, runState string, total int) projectioncontract.RuntimeTimelineItem {
	omitted := total - streamTimelineHeadItemKeep - streamTimelineTailItemKeep
	return projectioncontract.RuntimeTimelineItem{
		Source:    "step_stream",
		Type:      "omitted",
		ID:        step.ID + "/stream/omitted",
		RunState:  runState,
		StepID:    step.ID,
		StepState: step.State,
		Summary:   "omitted " + strconv.Itoa(omitted) + " stream chunks",
		Data: map[string]any{
			"omitted": omitted,
			"total":   total,
		},
	}
}

// source: step_stream_timeline_items.go

func streamTimelineEventItem(step projectioncontract.StepView, runState string, stepOccurredAt string, idx int, event map[string]any) projectioncontract.RuntimeTimelineItem {
	bounded := boundedStreamTimelineEvent(event)
	eventType, _ := event["type"].(string)
	if eventType == "" {
		eventType = "delta"
	}
	occurredAt := streamEventOccurredAt(event)
	if occurredAt == "" {
		occurredAt = stepOccurredAt
	}
	return projectioncontract.RuntimeTimelineItem{
		Source:     "step_stream",
		Type:       eventType,
		ID:         step.ID + "/stream/" + strconv.Itoa(idx+1),
		RunState:   runState,
		StepID:     step.ID,
		StepState:  step.State,
		Summary:    readModelStringValue(bounded["delta"]),
		OccurredAt: occurredAt,
		Data:       bounded,
	}
}

// source: timeline_summary.go

func timelineStepSummary(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	if stage := readModelStringValue(output["stage"]); stage != "" {
		return stage
	}
	if status := readModelStringValue(output["status"]); status != "" {
		return status
	}
	return ""
}

func memoryRecallTimelineSummary(trace projectioncontract.MemoryRecallTrace) string {
	count := len(trace.RetrievedMemoryIDs)
	if trace.Status == "degraded" && trace.Reason != "" {
		return "memory recall degraded: " + trace.Reason
	}
	if count == 0 {
		return "memory recall " + trace.Status
	}
	return "memory recall " + trace.Status + ": " + strconv.Itoa(count) + " memories"
}
