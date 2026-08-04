package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

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
