package orchestrator

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) NextActionForRun(ctx context.Context, runID common.RunID) (runtimemodel.NextAction, error) {
	if o == nil || runID == "" {
		return runtimemodel.NextAction{}, common.ErrInvalidArgument
	}
	run, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return runtimemodel.NextAction{}, err
	}
	if run.State == common.RunStateBlockedOnApproval && run.BlockingApprovalID != "" && o.approvals != nil {
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
		approval, err := o.approvals.Get(scopedCtx, run.BlockingApprovalID)
		if err != nil {
			return runtimemodel.NextAction{}, err
		}
		if action, ok := derivePendingApprovalNextAction(run, approval); ok {
			return action, nil
		}
	}
	return runtimemodel.NextAction{}, nil
}

func derivePendingApprovalNextAction(run *runmodel.Run, approval *approvalmodel.Approval) (runtimemodel.NextAction, bool) {
	if run == nil || approval == nil {
		return runtimemodel.NextAction{}, false
	}
	if run.ID == "" || run.WorkspaceID == "" || run.State != common.RunStateBlockedOnApproval || run.BlockingApprovalID == "" {
		return runtimemodel.NextAction{}, false
	}
	if approval.ID != run.BlockingApprovalID || approval.WorkspaceID != run.WorkspaceID || approval.RunID != run.ID || approval.State != common.ApprovalStatePending {
		return runtimemodel.NextAction{}, false
	}
	return runtimemodel.NextAction{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Type:        runtimemodel.NextActionTypeRequestApproval,
		SubjectType: "approval",
		SubjectID:   string(approval.ID),
		Stage:       "approval",
		StageMode:   "governance",
		RequiredOutput: map[string]any{
			"approvalId": string(approval.ID),
			"action":     approval.Action,
		},
		AllowedActions:   []string{"approve", "reject"},
		ForbiddenActions: []string{runtimemodel.NextActionTypeStateChange},
		Reason:           "pending approval blocks run",
	}.Normalized(), true
}
