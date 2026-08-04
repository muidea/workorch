package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func streamTimelineOmittedItem(step readmodelcore.StepView, runState string, total int) readmodelcore.RuntimeTimelineItem {
	omitted := total - streamTimelineHeadItemKeep - streamTimelineTailItemKeep
	return readmodelcore.RuntimeTimelineItem{
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
