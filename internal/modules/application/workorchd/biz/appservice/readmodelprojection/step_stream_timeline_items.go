package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func streamTimelineEventItem(step readmodelcore.StepView, runState string, stepOccurredAt string, idx int, event map[string]any) readmodelcore.RuntimeTimelineItem {
	bounded := boundedStreamTimelineEvent(event)
	eventType, _ := event["type"].(string)
	if eventType == "" {
		eventType = "delta"
	}
	occurredAt := streamEventOccurredAt(event)
	if occurredAt == "" {
		occurredAt = stepOccurredAt
	}
	return readmodelcore.RuntimeTimelineItem{
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
