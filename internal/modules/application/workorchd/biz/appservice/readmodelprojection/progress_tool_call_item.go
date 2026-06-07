package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func toolCallProgressItem(item readmodelcore.ToolCall, recoveredHistoricalFailures bool) readmodelcore.ProgressItem {
	return readmodelcore.ProgressItem{
		ID:           item.ID,
		Kind:         "tool_call",
		Title:        firstNonEmptyTimelineString(item.ToolName, item.ID),
		State:        RecoveredProgressItemState(item.State, recoveredHistoricalFailures),
		EvidenceRefs: []string{item.ID, item.StepID},
	}
}
