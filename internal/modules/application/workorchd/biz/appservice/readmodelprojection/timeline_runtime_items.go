package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func stepRuntimeTimelineItem(step projectioncontract.StepView, runState string, occurredAt string) projectioncontract.RuntimeTimelineItem {
	return projectioncontract.RuntimeTimelineItem{
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

func checkpointRuntimeTimelineItem(checkpoint projectioncontract.RuntimeCheckpoint) projectioncontract.RuntimeTimelineItem {
	return projectioncontract.RuntimeTimelineItem{
		Source:     "checkpoint",
		Type:       checkpoint.CheckpointType,
		ID:         checkpoint.ID,
		RunState:   checkpoint.RunStateSnapshot,
		StepID:     checkpoint.CurrentStepID,
		OccurredAt: checkpoint.OccurredAt,
		Data:       cloneReadModelMap(checkpoint.Data),
	}
}
