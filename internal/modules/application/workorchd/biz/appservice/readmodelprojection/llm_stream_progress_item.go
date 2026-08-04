package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/common"
)

func streamProgressTimelineItemFromPayload(id string, topic string, occurredAt string, stepID string, payload map[string]any, runState string) readmodelcore.RuntimeTimelineItem {
	projectedStepID := firstNonEmptyTimelineString(stepID, readModelStringValue(payload[recordpayload.PayloadKeyStepID]))
	itemType := firstNonEmptyTimelineString(readModelStringValue(payload[recordpayload.PayloadKeyStatus]), "progress")
	summary := readModelStringValue(payload[recordpayload.PayloadKeySummary])
	if summary == "" {
		summary = streamProgressSummaryFromPayload(payload)
	}
	displayState := firstNonEmptyTimelineString(readModelStringValue(payload[recordpayload.PayloadKeyStatus]), runState)
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
