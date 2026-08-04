package readmodelprojection

import (
	"strconv"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

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
