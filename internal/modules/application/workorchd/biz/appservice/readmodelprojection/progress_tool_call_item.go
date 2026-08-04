package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func toolCallProgressItem(item projectioncontract.ToolCall, recoveredHistoricalFailures bool) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "tool_call",
		Title:        firstNonEmptyTimelineString(item.ToolName, item.ID),
		State:        RecoveredProgressItemState(item.State, recoveredHistoricalFailures),
		EvidenceRefs: []string{item.ID, item.StepID},
	}
}
