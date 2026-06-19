package orchestrator

import (
	"context"
	"fmt"
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) applyApprovedPendingPatchApproval(
	ctx context.Context,
	run *runmodel.Run,
	approval *approvalmodel.Approval,
) (map[string]any, error) {
	if o == nil || run == nil || approval == nil {
		return nil, nil
	}
	approvalKind := strings.TrimSpace(stringFromMap(approval.Context, "kind"))
	if approvalKind != "patch_approval_required" {
		return nil, nil
	}
	if approval.State != common.ApprovalStateApproved {
		return nil, common.ErrBlocked
	}
	if approval.RunID != run.ID || (approval.WorkspaceID != "" && approval.WorkspaceID != run.WorkspaceID) {
		return nil, common.ErrConflict
	}
	if approval.StepID == "" || o.steps == nil || o.workspaces == nil {
		return nil, common.ErrInvalidArgument
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
	step, err := o.steps.Get(ctx, approval.StepID)
	if err != nil {
		return nil, err
	}
	patchContext := pendingPatchApprovalContextFromBlockedStep(step)
	if strings.TrimSpace(stringFromMap(patchContext, "kind")) != "patch_approval_required" {
		patchContext = approval.Context
	}
	if applied := o.patchManager().approvedPatchApplicationSummary(ctx, run, approval); len(applied) > 0 {
		annotateApprovedPatchApplicationOnBlockedStep(step, applied)
		if err := o.steps.Update(ctx, step); err != nil {
			return nil, err
		}
		return applied, nil
	}
	patches := cloneMapSlice(mapSliceFromValue(patchContext["patchRequests"]))
	if len(patches) == 0 {
		return nil, common.ErrInvalidArgument
	}
	for _, patch := range patches {
		patch["approvalId"] = string(approval.ID)
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil {
		return nil, err
	}
	output := map[string]any{
		"status":             "completed",
		"stage":              "approved_patch_apply",
		stepOutputKeyPatches: patches,
	}
	toolCall := &runmodel.ToolCall{
		ID:          common.ToolCallID(fmt.Sprintf("%s/tool-call/approved-patch-apply", approval.StepID)),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      approval.StepID,
		ToolName:    "patch/apply",
		State:       common.ToolCallStateCompleted,
		Output:      output,
	}
	if err := o.patchManager().apply(ctx, run, step, toolCall, output, workspace.DeliverableDirectory); err != nil {
		return nil, err
	}
	applied := approvedPatchApplicationSummaryFromOutput(approval, output)
	annotateApprovedPatchApplicationOnBlockedStep(step, applied)
	if err := o.steps.Update(ctx, step); err != nil {
		return nil, err
	}
	return applied, nil
}

func (p *patchSubsystem) approvedPatchApplicationSummary(ctx context.Context, run *runmodel.Run, approval *approvalmodel.Approval) map[string]any {
	if p == nil || p.evidence.Patches == nil || run == nil || approval == nil || approval.ID == "" {
		return nil
	}
	items, err := p.evidence.Patches.ListByRun(ctx, run.ID)
	if err != nil {
		return nil
	}
	patches := make([]map[string]any, 0)
	for _, item := range items {
		if item == nil ||
			item.ApprovalID != approval.ID ||
			item.RunID != run.ID ||
			item.StepID != approval.StepID ||
			item.State != common.PatchStateApplied {
			continue
		}
		patches = append(patches, map[string]any{
			"patchId":    string(item.ID),
			"targetRef":  item.TargetRef,
			"patchType":  item.PatchType,
			"state":      string(item.State),
			"beforeHash": item.BeforeHash,
			"afterHash":  item.AfterHash,
		})
	}
	if len(patches) == 0 {
		return nil
	}
	return map[string]any{
		"approvalId": string(approval.ID),
		"state":      "already_applied",
		"patches":    patches,
	}
}

func approvedPatchApplicationSummaryFromOutput(approval *approvalmodel.Approval, output map[string]any) map[string]any {
	patches := make([]map[string]any, 0)
	for _, item := range mapSliceFromValue(output[stepOutputKeyPatches]) {
		patches = append(patches, map[string]any{
			"patchId":    stringFromMap(item, "patchId"),
			"targetRef":  stringFromMap(item, "targetRef"),
			"patchType":  stringFromMap(item, "patchType"),
			"state":      stringFromMap(item, "state"),
			"beforeHash": stringFromMap(item, "beforeHash"),
			"afterHash":  stringFromMap(item, "afterHash"),
		})
	}
	ret := map[string]any{
		"state":   "applied",
		"patches": patches,
	}
	if approval != nil {
		ret["approvalId"] = string(approval.ID)
	}
	return ret
}

func annotateApprovedPatchApplicationOnBlockedStep(step *runmodel.Step, application map[string]any) {
	if step == nil || len(application) == 0 {
		return
	}
	if step.Output == nil {
		step.Output = map[string]any{}
	}
	step.Output["approvedPatchApplication"] = cloneStepPayload(application)
	step.Output["pendingPatchApprovalResolved"] = true
	nextInput := cloneStepPayload(mapFromValue(step.Output[stepOutputKeyNextInput]))
	if nextInput == nil {
		nextInput = map[string]any{}
	}
	delete(nextInput, "pendingPatchApproval")
	nextInput["approvedPatchApplication"] = cloneStepPayload(application)
	step.Output[stepOutputKeyNextInput] = nextInput
}

func pendingPatchApprovalContextFromBlockedStep(step *runmodel.Step) map[string]any {
	if step == nil || step.Output == nil {
		return nil
	}
	for _, key := range []string{
		stepOutputKeyBlockApprovalContext,
		"pendingPatchApproval",
	} {
		if context := mapFromValue(step.Output[key]); len(context) > 0 {
			return context
		}
	}
	nextInput := mapFromValue(step.Output[stepOutputKeyNextInput])
	if len(nextInput) == 0 {
		return nil
	}
	return mapFromValue(nextInput["pendingPatchApproval"])
}
