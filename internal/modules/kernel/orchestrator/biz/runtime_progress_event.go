package orchestrator

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) RecordRuntimeStepProgress(ctx context.Context, progress runtimemodel.RuntimeStepExecutionProgressed) error {
	if o == nil || o.runs == nil || o.steps == nil || progress.RunID == "" || progress.StepID == "" {
		return common.ErrInvalidArgument
	}
	run, err := o.runs.Get(ctx, progress.RunID)
	if err != nil {
		return err
	}
	step, err := o.steps.Get(ctx, progress.StepID)
	if err != nil {
		return err
	}
	if step.RunID != run.ID {
		return common.ErrConflict
	}
	event := strings.TrimSpace(progress.Event)
	if event == "" {
		event = "runtime_step_progress"
	}
	data := map[string]any{}
	for key, value := range progress.Data {
		if key == "" || value == nil {
			continue
		}
		data[key] = value
	}
	if progress.RequestID != "" {
		data["requestId"] = progress.RequestID
	}
	if progress.Stage != "" {
		data["stage"] = progress.Stage
	}
	if progress.Summary != "" {
		data["summary"] = progress.Summary
	}
	if progress.OccurredAt != "" {
		data["occurredAt"] = progress.OccurredAt
	}
	return o.checkpointManager().recordStepProgressCheckpoint(ctx, run, step, event, data)
}
