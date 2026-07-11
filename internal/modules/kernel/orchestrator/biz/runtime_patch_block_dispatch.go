package orchestrator

import (
	"context"
	"fmt"

	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) applyPatchCandidatesThroughRuntimeBlocks(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	workspace *workspaceModel.Workspace,
) (bool, map[string]any, error) {
	requests, err := runtimePatchRequests(output)
	if err != nil || len(requests) == 0 {
		return false, output, err
	}
	if o == nil || o.runtime == nil || run == nil || step == nil || workspace == nil {
		return true, output, common.ErrInvalidArgument
	}
	if len(requests) != 1 {
		return true, output, fmt.Errorf("%w: direct patch candidate runtime dispatch requires exactly one patch request", common.ErrInvalidArgument)
	}
	patchInput := cloneStepPayload(requests[0].Extra)
	if len(patchInput) == 0 {
		patchInput = map[string]any{
			"patchType":       requests[0].PatchType,
			"targetRef":       requests[0].TargetRef,
			"content":         requests[0].Content,
			"diff":            requests[0].Diff,
			"oldText":         requests[0].OldText,
			"newText":         requests[0].NewText,
			"baseHash":        requests[0].BaseHash,
			"occurrence":      requests[0].Occurrence,
			"approvalId":      string(requests[0].ApprovalID),
			"coverageTargets": requests[0].CoverageTargets,
		}
	}
	patchStep := *step
	patchStep.Kind = "patch/apply"
	patchStep.Input = patchInput
	result, err := o.runtime.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID:                   run.WorkspaceID,
		WorkspaceDeliverableDirectory: workspace.DeliverableDirectory,
		RunID:                         run.ID,
		StepID:                        step.ID,
		Kind:                          "patch/apply",
		Input:                         patchInput,
	})
	if err != nil {
		if result.Output != nil {
			return true, result.Output, err
		}
		return true, output, err
	}
	blockOutput := result.Output
	if blockOutput == nil {
		blockOutput = map[string]any{}
	}
	if err := o.patchManager().apply(ctx, run, &patchStep, toolCall, blockOutput, workspace.DeliverableDirectory); err != nil {
		return true, blockOutput, err
	}
	for key, value := range blockOutput {
		output[key] = value
	}
	return true, output, nil
}
