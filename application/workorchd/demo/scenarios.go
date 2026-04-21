package demo

import (
	"context"
	"errors"
	"fmt"

	"github.com/muidea/workorch/application/workorchd/bootstrap"
	"github.com/muidea/workorch/application/workorchd/presenter"
	"github.com/muidea/workorch/internal/pkg/common"
)

func runRunnableSlice(ctx context.Context, rt *bootstrap.Runtime) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-phase1-runnable")
	runID := common.RunID("run-phase1-runnable")
	stepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := ensureWorkspace(ctx, rt, workspaceID, "phase1-runnable"); err != nil {
		return nil, err
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := ensureRunStarted(ctx, rt, runID); err != nil {
		return nil, err
	}

	return snapshot(ctx, rt, "runnable", workspaceID, runID, stepID, presenter.BlockingRefs{})
}

func runDecisionBlockedResumeSlice(ctx context.Context, rt *bootstrap.Runtime) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-phase1-blocked-decision")
	runID := common.RunID("run-phase1-blocked-decision")
	manualStepID := common.StepID("run-phase1-blocked-decision/manual-gate")
	ticketID := common.TicketID("ticket-phase1-blocked-decision")
	resumedStepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := ensureWorkspace(ctx, rt, workspaceID, "phase1-decision-blocked-resume"); err != nil {
		return nil, err
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := ensureDecisionTicket(ctx, rt, ticketID, workspaceID, runID, manualStepID, "phase1 manual decision gate"); err != nil {
		return nil, err
	}
	if err := ensureRunBlockedOnDecision(ctx, rt, runID, ticketID); err != nil {
		return nil, err
	}
	if err := ensureDecisionResolved(ctx, rt, ticketID); err != nil {
		return nil, err
	}
	if err := ensureRunResumed(ctx, rt, runID); err != nil {
		return nil, err
	}

	return snapshot(ctx, rt, "decision_blocked_resume", workspaceID, runID, resumedStepID, presenter.BlockingRefs{TicketID: ticketID})
}

func runApprovalBlockedResumeSlice(ctx context.Context, rt *bootstrap.Runtime) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-phase1-blocked-approval")
	runID := common.RunID("run-phase1-blocked-approval")
	manualStepID := common.StepID("run-phase1-blocked-approval/manual-approval")
	approvalID := common.ApprovalID("approval-phase1-blocked-approval")
	resumedStepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := ensureWorkspace(ctx, rt, workspaceID, "phase1-approval-blocked-resume"); err != nil {
		return nil, err
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := ensureApproval(ctx, rt, approvalID, workspaceID, runID, manualStepID, "phase1 manual approval gate"); err != nil {
		return nil, err
	}
	if err := ensureRunBlockedOnApproval(ctx, rt, runID, approvalID); err != nil {
		return nil, err
	}
	if err := ensureApprovalApproved(ctx, rt, approvalID); err != nil {
		return nil, err
	}
	if err := ensureRunResumed(ctx, rt, runID); err != nil {
		return nil, err
	}

	return snapshot(ctx, rt, "approval_blocked_resume", workspaceID, runID, resumedStepID, presenter.BlockingRefs{ApprovalID: approvalID})
}

func runApprovalRejectedTerminalSlice(ctx context.Context, rt *bootstrap.Runtime) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-phase1-rejected-approval")
	runID := common.RunID("run-phase1-rejected-approval")
	manualStepID := common.StepID("run-phase1-rejected-approval/manual-approval")
	approvalID := common.ApprovalID("approval-phase1-rejected-approval")

	if err := ensureWorkspace(ctx, rt, workspaceID, "phase1-approval-rejected-terminal"); err != nil {
		return nil, err
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := ensureApproval(ctx, rt, approvalID, workspaceID, runID, manualStepID, "phase1 manual approval rejection gate"); err != nil {
		return nil, err
	}
	if err := ensureRunBlockedOnApproval(ctx, rt, runID, approvalID); err != nil {
		return nil, err
	}
	if err := ensureApprovalRejected(ctx, rt, approvalID); err != nil {
		return nil, err
	}
	if err := ensureRunResumed(ctx, rt, runID); err != nil {
		return nil, err
	}

	return snapshot(ctx, rt, "approval_rejected_terminal", workspaceID, runID, "", presenter.BlockingRefs{ApprovalID: approvalID})
}

func ensureWorkspace(ctx context.Context, rt *bootstrap.Runtime, workspaceID common.WorkspaceID, title string) error {
	item, err := rt.GetWorkspaceByID(ctx, workspaceID)
	if err == nil {
		if item.Title != title {
			item.Title = title
		}
		if item.State != common.WorkspaceStateActive || item.ActiveRunID != "" {
			item.State = common.WorkspaceStateActive
			item.ActiveRunID = ""
			return rt.UpdateWorkspace(ctx, item)
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return rt.CreateWorkspace(ctx, workspaceID, title)
}

func ensureRun(ctx context.Context, rt *bootstrap.Runtime, runID common.RunID, workspaceID common.WorkspaceID) error {
	item, err := rt.GetRun(ctx, runID)
	if err == nil {
		if item.WorkspaceID != workspaceID {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return rt.CreateRun(ctx, runID, workspaceID)
}

func ensureRunStarted(ctx context.Context, rt *bootstrap.Runtime, runID common.RunID) error {
	item, err := rt.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if item.State == common.RunStateCompleted {
		return nil
	}
	if item.State == common.RunStateFailed {
		return common.ErrConflict
	}
	if item.State == common.RunStateBlockedOnDecision || item.State == common.RunStateBlockedOnApproval {
		return common.ErrConflict
	}
	return rt.StartRun(ctx, runID)
}

func ensureDecisionTicket(ctx context.Context, rt *bootstrap.Runtime, ticketID common.TicketID, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, reason string) error {
	item, err := rt.GetTicket(ctx, ticketID)
	if err == nil {
		if item.WorkspaceID != workspaceID || item.RunID != runID || item.StepID != stepID {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return rt.CreateTicket(ctx, ticketID, workspaceID, runID, stepID, reason)
}

func ensureRunBlockedOnDecision(ctx context.Context, rt *bootstrap.Runtime, runID common.RunID, ticketID common.TicketID) error {
	item, err := rt.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if item.State == common.RunStateBlockedOnDecision && item.BlockingTicketID == ticketID {
		return nil
	}
	if item.State == common.RunStateCompleted || item.State == common.RunStateFailed {
		return nil
	}
	return rt.BlockOnDecision(ctx, runID, ticketID)
}

func ensureDecisionResolved(ctx context.Context, rt *bootstrap.Runtime, ticketID common.TicketID) error {
	item, err := rt.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	if item.State == common.DecisionTicketStateResolved {
		return nil
	}
	return rt.ResolveTicket(
		ctx,
		common.ResolutionID("resolution-phase1-blocked-decision"),
		ticketID,
		"approve_path",
		map[string]any{"operator": "phase1-demo"},
	)
}

func ensureApproval(ctx context.Context, rt *bootstrap.Runtime, approvalID common.ApprovalID, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, reason string) error {
	item, err := rt.GetApproval(ctx, approvalID)
	if err == nil {
		if item.WorkspaceID != workspaceID || item.RunID != runID || item.StepID != stepID {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return rt.CreateApproval(ctx, approvalID, workspaceID, runID, stepID, reason)
}

func ensureRunBlockedOnApproval(ctx context.Context, rt *bootstrap.Runtime, runID common.RunID, approvalID common.ApprovalID) error {
	item, err := rt.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if item.State == common.RunStateBlockedOnApproval && item.BlockingApprovalID == approvalID {
		return nil
	}
	if item.State == common.RunStateCompleted || item.State == common.RunStateFailed {
		return nil
	}
	return rt.BlockOnApproval(ctx, runID, approvalID)
}

func ensureApprovalApproved(ctx context.Context, rt *bootstrap.Runtime, approvalID common.ApprovalID) error {
	item, err := rt.GetApproval(ctx, approvalID)
	if err != nil {
		return err
	}
	if item.State == common.ApprovalStateApproved {
		return nil
	}
	if item.State == common.ApprovalStateRejected {
		return common.ErrConflict
	}
	return rt.ApproveApproval(ctx, approvalID)
}

func ensureApprovalRejected(ctx context.Context, rt *bootstrap.Runtime, approvalID common.ApprovalID) error {
	item, err := rt.GetApproval(ctx, approvalID)
	if err != nil {
		return err
	}
	if item.State == common.ApprovalStateRejected {
		return nil
	}
	if item.State == common.ApprovalStateApproved {
		return common.ErrConflict
	}
	return rt.RejectApproval(ctx, approvalID)
}

func ensureRunResumed(ctx context.Context, rt *bootstrap.Runtime, runID common.RunID) error {
	item, err := rt.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if item.State == common.RunStateCompleted || item.State == common.RunStateFailed {
		return nil
	}
	if item.State != common.RunStateBlockedOnDecision && item.State != common.RunStateBlockedOnApproval {
		return common.ErrConflict
	}
	return rt.ResumeRun(ctx, runID)
}
