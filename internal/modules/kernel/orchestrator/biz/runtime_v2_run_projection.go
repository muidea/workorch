package orchestrator

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ActivateRuntimeV2Run 只投影 Runtime V2 continuation 所需的 Run/Workspace owner 状态，
// 不创建 V1 Step，也不进入 legacy dispatch/recovery。
func (o *RuntimeOrchestrator) ActivateRuntimeV2Run(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) error {
	if o == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	run, err := o.runtimeEventRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), runID)
	if err != nil {
		return err
	}
	if run.WorkspaceID != workspaceID {
		return common.ErrNotFound
	}
	if run.State == common.RunStateActive {
		return nil
	}
	if run.State != common.RunStateBlockedOnDecision && run.State != common.RunStateBlockedOnApproval {
		return common.ErrConflict
	}
	run.State = common.RunStateActive
	run.BlockingTicketID = ""
	run.BlockingApprovalID = ""
	run.FailureReason = ""
	_, err = o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, run, common.WorkspaceStateActive, nil)
	if err != nil {
		return err
	}
	return nil
}

// ProjectRuntimeV2Terminal 将 typed reducer terminal 投影到 Run/Workspace owner。
// V2 terminal 已经完成 evidence/coverage 判定，不再经过 V1 Result/DoD completion gate。
func (o *RuntimeOrchestrator) ProjectRuntimeV2Terminal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, action runtimecontract.RuntimeAction, terminal *runtimecontract.TerminalOutcome) error {
	if o == nil || workspaceID == "" || runID == "" || terminal == nil {
		return common.ErrInvalidArgument
	}
	run, err := o.runtimeEventRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), runID)
	if err != nil {
		return err
	}
	if run.WorkspaceID != workspaceID {
		return common.ErrNotFound
	}
	switch action {
	case runtimecontract.RuntimeActionComplete:
		if run.State == common.RunStateCompleted {
			return nil
		}
		if run.State != common.RunStateActive {
			return common.ErrConflict
		}
		run.State = common.RunStateCompleted
		run.BlockingTicketID = ""
		run.BlockingApprovalID = ""
		run.FailureReason = ""
		if strings.TrimSpace(run.EndedAt) == "" {
			run.EndedAt = runtimeEvidenceTimestamp()
		}
		if err := o.runs.Update(ctx, run); err != nil {
			return err
		}
		_, changed, err := o.runStateManager().releaseWorkspaceForTerminalRun(ctx, run, common.WorkspaceStateActive)
		if err != nil || !changed {
			return err
		}
		return nil
	case runtimecontract.RuntimeActionFail:
		if run.State == common.RunStateFailed {
			return nil
		}
		reason := strings.TrimSpace(terminal.Summary)
		if reason == "" {
			reason = strings.TrimSpace(terminal.Code)
		}
		return o.closeLoadedRun(ctx, run, common.RunStateFailed, reason)
	default:
		return common.ErrInvalidArgument
	}
}
