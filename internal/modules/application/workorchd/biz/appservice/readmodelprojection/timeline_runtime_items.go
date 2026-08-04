package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func stepRuntimeTimelineItem(step readmodelcore.StepView, runState string, occurredAt string) readmodelcore.RuntimeTimelineItem {
	return readmodelcore.RuntimeTimelineItem{
		Source:     "step",
		Type:       step.Kind,
		ID:         step.ID,
		RunState:   runState,
		StepID:     step.ID,
		StepState:  step.State,
		Summary:    TimelineStepSummary(step.Output),
		OccurredAt: occurredAt,
		Output:     boundedStreamStepOutput(step.Output),
	}
}

func checkpointRuntimeTimelineItem(checkpoint projectioncontract.RuntimeCheckpoint) readmodelcore.RuntimeTimelineItem {
	return readmodelcore.RuntimeTimelineItem{
		Source:     "checkpoint",
		Type:       checkpoint.CheckpointType,
		ID:         checkpoint.ID,
		RunState:   checkpoint.RunStateSnapshot,
		StepID:     checkpoint.CurrentStepID,
		OccurredAt: checkpoint.OccurredAt,
		Data:       cloneReadModelMap(checkpoint.Data),
	}
}
