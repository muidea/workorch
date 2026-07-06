package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func streamProgressTimelineItemFromPayload(id string, topic string, occurredAt string, stepID string, payload map[string]any, runState string) readmodelcore.RuntimeTimelineItem {
	projectedStepID := firstNonEmptyTimelineString(stepID, readModelStringValue(payload[events.PayloadKeyStepID]))
	itemType := firstNonEmptyTimelineString(readModelStringValue(payload[events.PayloadKeyStatus]), "progress")
	summary := readModelStringValue(payload[events.PayloadKeySummary])
	if summary == "" {
		summary = streamProgressSummaryFromPayload(payload)
	}
	displayState := firstNonEmptyTimelineString(readModelStringValue(payload[events.PayloadKeyStatus]), runState)
	switch displayState {
	case "completed":
		displayState = string(common.StepStateSucceeded)
	case "dispatched":
		displayState = string(common.StepStateExecuting)
	default:
		displayState = string(common.CanonicalStepState(common.StepState(displayState)))
	}
	return readmodelcore.RuntimeTimelineItem{
		Source:     "llm_stream_progress_event",
		Type:       itemType,
		ID:         firstNonEmptyTimelineString(id, projectedStepID+"/llm-stream-progress"),
		Topic:      topic,
		RunState:   displayState,
		StepID:     projectedStepID,
		StepState:  itemType,
		Summary:    summary,
		OccurredAt: occurredAt,
		Data:       payload,
	}
}
