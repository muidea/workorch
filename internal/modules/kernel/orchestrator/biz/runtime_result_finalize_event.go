package orchestrator

import (
	"context"
	"fmt"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) FinalizeRuntimeStepResult(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
	result runtimemodel.StepExecutionResult,
	execErr error,
) error {
	if o == nil || o.runs == nil || o.steps == nil || o.workspaces == nil || runID == "" || stepID == "" {
		return common.ErrInvalidArgument
	}
	run, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return err
	}
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		return err
	}
	if step.RunID != run.ID {
		return common.ErrConflict
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil {
		return err
	}
	toolCall, err := o.runtimeToolCallForStep(ctx, stepID)
	if err != nil {
		return err
	}
	return o.finalizeRuntimeStepResultWithWorkspace(ctx, run, step, toolCall, workspace, result, execErr)
}

func (o *RuntimeOrchestrator) runtimeEventRun(ctx context.Context, runID common.RunID) (*runModel.Run, error) {
	if o == nil || o.runs == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		return o.runs.GetByWorkspaceID(ctx, workspaceID, runID)
	}
	return o.runs.Get(ctx, runID)
}

func (o *RuntimeOrchestrator) runtimeToolCallForStep(ctx context.Context, stepID common.StepID) (*runModel.ToolCall, error) {
	if o == nil || o.evidence.ToolCalls == nil || stepID == "" {
		return &runModel.ToolCall{}, nil
	}
	id := common.ToolCallID(fmt.Sprintf("%s/tool-call/runtime-execute", stepID))
	toolCall, err := o.evidence.ToolCalls.Get(ctx, id)
	if err == nil {
		return toolCall, nil
	}
	if err != common.ErrNotFound {
		return nil, err
	}
	items, err := o.evidence.ToolCalls.ListByStep(ctx, stepID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return &runModel.ToolCall{}, nil
	}
	return items[0], nil
}
