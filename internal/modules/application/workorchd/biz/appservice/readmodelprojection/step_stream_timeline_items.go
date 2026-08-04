package readmodelprojection

import (
	"strconv"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

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
