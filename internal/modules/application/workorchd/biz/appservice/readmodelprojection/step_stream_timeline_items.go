package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func streamTimelineEventItem(step presenter.StepView, runState string, stepOccurredAt string, idx int, event map[string]any) readmodelcore.RuntimeTimelineItem {
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
		Summary:    readModelStringValue(event["delta"]),
		OccurredAt: occurredAt,
		Data:       cloneReadModelMap(event),
	}
}
