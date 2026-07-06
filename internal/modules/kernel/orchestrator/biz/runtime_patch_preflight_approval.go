package orchestrator

import (
	"context"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) blockPatchApplyForApprovalBeforeRuntime(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	workspace *workspaceModel.Workspace,
	input map[string]any,
) (bool, error) {
	if o == nil || run == nil || step == nil || strings.TrimSpace(step.Kind) != "patch/apply" {
		return false, nil
	}
	output := map[string]any{
		"status": "blocked_on_approval",
		"stage":  "patch_preflight_approval",
		stepOutputKeyPatches: []map[string]any{
			cloneStepPayload(input),
		},
	}
	approvalSpec, err := o.patchManager().approvalBlockSpec(ctx, run.ID, runDialogLanguage(run), output)
	if err != nil || approvalSpec.Reason == "" {
		return false, err
	}
	if approvalSpec.ApprovalID == "" {
		approvalSpec.ApprovalID = common.ApprovalID(runtimeChildID(step.ID, "approval"))
	}
	applyPatchApprovalBlockOutput(output, approvalSpec)
	output[stepOutputKeyBlockApprovalID] = string(approvalSpec.ApprovalID)
	output["preflightApproval"] = true
	if err := o.createApproval(ctx, run, step, approvalSpec.ApprovalID, approvalSpec.Reason, approvalSpec.Action, approvalSpec.Context); err != nil {
		return true, err
	}
	if err := o.finalizeGovernedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
		return true, err
	}
	return true, o.blockOnApprovalAtStep(ctx, run, workspace, approvalSpec.ApprovalID, step.ID)
}
