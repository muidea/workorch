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
		_, err = o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, run, common.WorkspaceStateActive, nil)
		return err
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
	if action != runtimecontract.RuntimeActionComplete && action != runtimecontract.RuntimeActionFail {
		return common.ErrInvalidArgument
	}
	run, err := o.runtimeEventRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), runID)
	if err != nil {
		return err
	}
	if run.WorkspaceID != workspaceID {
		return common.ErrNotFound
	}
	if run.State != common.RunStateCompleted && run.State != common.RunStateFailed {
		if run.RuntimeContractVersion != runtimecontract.RuntimeContractVersionV2 || run.RuntimeState == nil {
			return common.ErrConflict
		}
		state := run.RuntimeState.Clone()
		if err := state.Validate(); err != nil || state.Decision.Action != action || state.Decision.Terminal == nil ||
			strings.TrimSpace(state.Decision.Terminal.Code) != strings.TrimSpace(terminal.Code) ||
			strings.TrimSpace(state.Decision.Terminal.Summary) != strings.TrimSpace(terminal.Summary) {
			return common.ErrConflict
		}
	}
	switch action {
	case runtimecontract.RuntimeActionComplete:
		if run.State == common.RunStateCompleted {
			// Idempotent retry: release is a no-op when ActiveRun already cleared or
			// owned by another live run; never rewrite completed Run authority.
			_, _, releaseErr := o.runStateManager().releaseWorkspaceForTerminalRun(ctx, run, common.WorkspaceStateActive)
			return releaseErr
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
		_, _, err := o.runStateManager().releaseWorkspaceForTerminalRun(ctx, run, common.WorkspaceStateActive)
		if err != nil {
			return err
		}
		return o.runs.Update(ctx, run)
	case runtimecontract.RuntimeActionFail:
		if run.State == common.RunStateFailed {
			_, _, releaseErr := o.runStateManager().releaseWorkspaceForTerminalRun(ctx, run, common.WorkspaceStateActive)
			return releaseErr
		}
		code := strings.TrimSpace(terminal.Code)
		reason := strings.TrimSpace(terminal.Summary)
		if code != "" && reason != "" {
			reason = code + ": " + reason
		} else if reason == "" {
			reason = code
		}
		return o.closeLoadedRun(ctx, run, common.RunStateFailed, reason)
	}
	return common.ErrInvalidArgument
}
