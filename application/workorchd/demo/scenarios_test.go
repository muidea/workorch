package demo

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/application/workorchd/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEnsureRunStartedReturnsConflictWhenRunBlocked(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-blocked-run")
	runID := common.RunID("run-error-blocked-run")
	approvalID := common.ApprovalID("approval-error-blocked-run")
	stepID := common.StepID("run-error-blocked-run/manual-approval")

	if err := ensureWorkspace(ctx, rt, workspaceID, "error blocked run"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := ensureApproval(ctx, rt, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.BlockOnApproval(ctx, runID, approvalID); err != nil {
		t.Fatalf("block on approval: %v", err)
	}

	err := ensureRunStarted(ctx, rt, runID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureRunStartedReturnsNilWhenRunAlreadyCompleted(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-completed-run")
	runID := common.RunID("run-completed-run")

	if err := ensureWorkspace(ctx, rt, workspaceID, "completed run"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := rt.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if err := ensureRunStarted(ctx, rt, runID); err != nil {
		t.Fatalf("expected nil for completed run, got %v", err)
	}
}

func TestEnsureRunResumedReturnsConflictWhenRunNotBlocked(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-resume")
	runID := common.RunID("run-error-resume")

	if err := ensureWorkspace(ctx, rt, workspaceID, "error resume"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}

	err := ensureRunResumed(ctx, rt, runID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureRunResumedReturnsBlockedWhenDecisionNotResolved(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-unresolved-decision")
	runID := common.RunID("run-error-unresolved-decision")
	ticketID := common.TicketID("ticket-error-unresolved-decision")
	stepID := common.StepID("run-error-unresolved-decision/manual-gate")

	if err := ensureWorkspace(ctx, rt, workspaceID, "error unresolved decision"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := ensureDecisionTicket(ctx, rt, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}
	if err := rt.BlockOnDecision(ctx, runID, ticketID); err != nil {
		t.Fatalf("block on decision: %v", err)
	}

	err := ensureRunResumed(ctx, rt, runID)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
}

func TestEnsureRunResumedReturnsNilWhenRunAlreadyFailedByRejectedApproval(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-failed-run")
	runID := common.RunID("run-failed-run")
	approvalID := common.ApprovalID("approval-failed-run")
	stepID := common.StepID("run-failed-run/manual-approval")
	runtimeStepID := common.StepID("run-failed-run/step-1")

	if err := ensureWorkspace(ctx, rt, workspaceID, "failed run"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := ensureApproval(ctx, rt, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.BlockOnApproval(ctx, runID, approvalID); err != nil {
		t.Fatalf("block on approval: %v", err)
	}
	if err := rt.RejectApproval(ctx, approvalID); err != nil {
		t.Fatalf("reject approval: %v", err)
	}
	if err := rt.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected approval run: %v", err)
	}

	if err := ensureRunResumed(ctx, rt, runID); err != nil {
		t.Fatalf("expected nil for failed run, got %v", err)
	}

	runItem, err := rt.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("get failed run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("expected failed run state, got %s", runItem.State)
	}
	if _, err := rt.GetStep(ctx, runtimeStepID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no runtime step after rejected approval, got %v", err)
	}
}

func TestEnsureRunReturnsConflictWhenWorkspaceMismatch(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-run-match")
	otherWorkspaceID := common.WorkspaceID("ws-error-run-mismatch")
	runID := common.RunID("run-error-run-mismatch")

	if err := ensureWorkspace(ctx, rt, workspaceID, "error run match"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureWorkspace(ctx, rt, otherWorkspaceID, "error run mismatch"); err != nil {
		t.Fatalf("ensure other workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}

	err := ensureRun(ctx, rt, runID, otherWorkspaceID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureApprovalRejectedReturnsConflictWhenAlreadyApproved(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-approval-approved")
	runID := common.RunID("run-error-approval-approved")
	approvalID := common.ApprovalID("approval-error-approval-approved")
	stepID := common.StepID("run-error-approval-approved/manual-approval")

	if err := ensureWorkspace(ctx, rt, workspaceID, "error approval approved"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := ensureApproval(ctx, rt, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.ApproveApproval(ctx, approvalID); err != nil {
		t.Fatalf("approve approval: %v", err)
	}

	err := ensureApprovalRejected(ctx, rt, approvalID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureDecisionTicketReturnsConflictWhenBindingMismatch(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-ticket")
	runID := common.RunID("run-error-ticket")
	ticketID := common.TicketID("ticket-error-ticket")
	stepID := common.StepID("run-error-ticket/manual-gate")
	otherStepID := common.StepID("run-error-ticket/manual-gate-other")

	if err := ensureWorkspace(ctx, rt, workspaceID, "error ticket"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := ensureDecisionTicket(ctx, rt, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}

	err := ensureDecisionTicket(ctx, rt, ticketID, workspaceID, runID, otherStepID, "manual decision")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureApprovalApprovedReturnsConflictWhenAlreadyRejected(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-approval")
	runID := common.RunID("run-error-approval")
	approvalID := common.ApprovalID("approval-error-approval")
	stepID := common.StepID("run-error-approval/manual-approval")

	if err := ensureWorkspace(ctx, rt, workspaceID, "error approval"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := ensureApproval(ctx, rt, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.RejectApproval(ctx, approvalID); err != nil {
		t.Fatalf("reject approval: %v", err)
	}

	err := ensureApprovalApproved(ctx, rt, approvalID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureApprovalReturnsConflictWhenBindingMismatch(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-approval-binding")
	runID := common.RunID("run-error-approval-binding")
	approvalID := common.ApprovalID("approval-error-approval-binding")
	stepID := common.StepID("run-error-approval-binding/manual-approval")
	otherStepID := common.StepID("run-error-approval-binding/manual-approval-other")

	if err := ensureWorkspace(ctx, rt, workspaceID, "error approval binding"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := ensureRun(ctx, rt, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := ensureApproval(ctx, rt, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}

	err := ensureApproval(ctx, rt, approvalID, workspaceID, runID, otherStepID, "manual approval")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}
