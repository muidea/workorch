package appservice

import (
	"context"
	"errors"
	"testing"

	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEnsureRunStartedReturnsConflictWhenRunBlocked(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-blocked-run")
	runID := common.RunID("run-error-blocked-run")
	approvalID := common.ApprovalID("approval-error-blocked-run")
	stepID := common.StepID("run-error-blocked-run/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error blocked run"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.EnsureRunBlockedOnApproval(ctx, runID, approvalID); err != nil {
		t.Fatalf("block on approval: %v", err)
	}

	err := svc.EnsureRunStarted(ctx, runID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureRunStartedReturnsNilWhenRunAlreadyCompleted(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-completed-run")
	runID := common.RunID("run-completed-run")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "completed run"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := rt.EnsureRunStarted(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if err := svc.EnsureRunStarted(ctx, runID); err != nil {
		t.Fatalf("expected nil for completed run, got %v", err)
	}
}

func TestEnsureRunStartedReturnsConflictWhenRunAlreadyFailed(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	svc := NewPhase1Service(session.Runtime)

	workspaceID := common.WorkspaceID("ws-failed-start-run")
	runID := common.RunID("run-failed-start-run")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "failed start run"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := session.Bootstrap.Exports().Services.RunService.FailRun(ctx, runmodule.FailRunCommand{
		RunID:  runID,
		Reason: "manual fail",
	}); err != nil {
		t.Fatalf("fail run: %v", err)
	}

	err := svc.EnsureRunStarted(ctx, runID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureRunResumedReturnsConflictWhenRunNotBlocked(t *testing.T) {
	svc, _ := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-resume")
	runID := common.RunID("run-error-resume")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error resume"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}

	err := svc.EnsureRunResumed(ctx, runID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureRunResumedReturnsBlockedWhenDecisionNotResolved(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-unresolved-decision")
	runID := common.RunID("run-error-unresolved-decision")
	ticketID := common.TicketID("ticket-error-unresolved-decision")
	stepID := common.StepID("run-error-unresolved-decision/manual-gate")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error unresolved decision"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}
	if err := rt.EnsureRunBlockedOnDecision(ctx, runID, ticketID); err != nil {
		t.Fatalf("block on decision: %v", err)
	}

	err := svc.EnsureRunResumed(ctx, runID)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
}

func TestEnsureRunResumedReturnsNilWhenRunAlreadyFailedByRejectedApproval(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-failed-run")
	runID := common.RunID("run-failed-run")
	approvalID := common.ApprovalID("approval-failed-run")
	stepID := common.StepID("run-failed-run/manual-approval")
	runtimeStepID := common.StepID("run-failed-run/step-1")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "failed run"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.EnsureRunBlockedOnApproval(ctx, runID, approvalID); err != nil {
		t.Fatalf("block on approval: %v", err)
	}
	if err := rt.EnsureApprovalRejected(ctx, approvalID); err != nil {
		t.Fatalf("reject approval: %v", err)
	}
	if err := rt.EnsureRunResumed(ctx, runID); err != nil {
		t.Fatalf("resume rejected approval run: %v", err)
	}

	if err := svc.EnsureRunResumed(ctx, runID); err != nil {
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

func TestEnsureRunResumedReturnsNilWhenRunAlreadyCompleted(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-completed-resume-run")
	runID := common.RunID("run-completed-resume-run")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "completed resume run"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := rt.EnsureRunStarted(ctx, runID); err != nil {
		t.Fatalf("ensure run started: %v", err)
	}

	if err := svc.EnsureRunResumed(ctx, runID); err != nil {
		t.Fatalf("expected nil for completed run, got %v", err)
	}
}

func TestEnsureRunBlockedOnDecisionReturnsNilWhenAlreadyBlockedOrTerminal(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-blocked-decision-repeat")
	runID := common.RunID("run-blocked-decision-repeat")
	ticketID := common.TicketID("ticket-blocked-decision-repeat")
	stepID := common.StepID("run-blocked-decision-repeat/manual-gate")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "blocked decision repeat"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}
	if err := rt.EnsureRunBlockedOnDecision(ctx, runID, ticketID); err != nil {
		t.Fatalf("ensure blocked on decision: %v", err)
	}
	if err := svc.EnsureRunBlockedOnDecision(ctx, runID, ticketID); err != nil {
		t.Fatalf("expected nil for same blocked decision, got %v", err)
	}
	if err := rt.EnsureResolvedTicket(ctx, common.ResolutionID("resolution-blocked-decision-repeat"), ticketID, "approve_path", map[string]any{"operator": "phase1-demo"}); err != nil {
		t.Fatalf("ensure resolved ticket: %v", err)
	}
	if err := rt.EnsureRunResumed(ctx, runID); err != nil {
		t.Fatalf("ensure run resumed: %v", err)
	}
	if err := svc.EnsureRunBlockedOnDecision(ctx, runID, ticketID); err != nil {
		t.Fatalf("expected nil for completed run, got %v", err)
	}
}

func TestEnsureRunBlockedOnApprovalReturnsNilWhenAlreadyBlockedOrTerminal(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-blocked-approval-repeat")
	runID := common.RunID("run-blocked-approval-repeat")
	approvalID := common.ApprovalID("approval-blocked-approval-repeat")
	stepID := common.StepID("run-blocked-approval-repeat/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "blocked approval repeat"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.EnsureRunBlockedOnApproval(ctx, runID, approvalID); err != nil {
		t.Fatalf("ensure blocked on approval: %v", err)
	}
	if err := svc.EnsureRunBlockedOnApproval(ctx, runID, approvalID); err != nil {
		t.Fatalf("expected nil for same blocked approval, got %v", err)
	}
	if err := rt.EnsureApprovalApproved(ctx, approvalID); err != nil {
		t.Fatalf("ensure approval approved: %v", err)
	}
	if err := rt.EnsureRunResumed(ctx, runID); err != nil {
		t.Fatalf("ensure run resumed: %v", err)
	}
	if err := svc.EnsureRunBlockedOnApproval(ctx, runID, approvalID); err != nil {
		t.Fatalf("expected nil for completed run, got %v", err)
	}
}

func TestEnsureDecisionResolvedReturnsNilWhenTicketAlreadyResolved(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-decision-resolved")
	runID := common.RunID("run-decision-resolved")
	ticketID := common.TicketID("ticket-decision-resolved")
	stepID := common.StepID("run-decision-resolved/manual-gate")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "decision resolved"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}
	if err := rt.EnsureResolvedTicket(ctx, common.ResolutionID("resolution-decision-resolved"), ticketID, "approve_path", map[string]any{"operator": "phase1-demo"}); err != nil {
		t.Fatalf("ensure resolved ticket: %v", err)
	}

	if err := svc.EnsureDecisionResolved(ctx, ticketID); err != nil {
		t.Fatalf("expected nil for already resolved ticket, got %v", err)
	}
}

func TestEnsureRunReturnsConflictWhenWorkspaceMismatch(t *testing.T) {
	svc, _ := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-run-match")
	otherWorkspaceID := common.WorkspaceID("ws-error-run-mismatch")
	runID := common.RunID("run-error-run-mismatch")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error run match"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureWorkspace(ctx, otherWorkspaceID, "error run mismatch"); err != nil {
		t.Fatalf("ensure other workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}

	err := svc.EnsureRun(ctx, runID, otherWorkspaceID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureApprovalRejectedReturnsConflictWhenAlreadyApproved(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-approval-approved")
	runID := common.RunID("run-error-approval-approved")
	approvalID := common.ApprovalID("approval-error-approval-approved")
	stepID := common.StepID("run-error-approval-approved/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error approval approved"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.EnsureApprovalApproved(ctx, approvalID); err != nil {
		t.Fatalf("approve approval: %v", err)
	}

	err := svc.EnsureApprovalRejected(ctx, approvalID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureApprovalRejectedReturnsNilWhenAlreadyRejected(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-approval-rejected")
	runID := common.RunID("run-error-approval-rejected")
	approvalID := common.ApprovalID("approval-error-approval-rejected")
	stepID := common.StepID("run-error-approval-rejected/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error approval rejected"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.EnsureApprovalRejected(ctx, approvalID); err != nil {
		t.Fatalf("ensure approval rejected: %v", err)
	}

	if err := svc.EnsureApprovalRejected(ctx, approvalID); err != nil {
		t.Fatalf("expected nil for already rejected approval, got %v", err)
	}
}

func TestEnsureDecisionTicketReturnsConflictWhenBindingMismatch(t *testing.T) {
	svc, _ := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-ticket")
	runID := common.RunID("run-error-ticket")
	ticketID := common.TicketID("ticket-error-ticket")
	stepID := common.StepID("run-error-ticket/manual-gate")
	otherStepID := common.StepID("run-error-ticket/manual-gate-other")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error ticket"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}

	err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, otherStepID, "manual decision")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureDecisionTicketReturnsConflictWhenWorkspaceOrRunBindingMismatch(t *testing.T) {
	svc, _ := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-ticket-binding")
	otherWorkspaceID := common.WorkspaceID("ws-error-ticket-binding-other")
	runID := common.RunID("run-error-ticket-binding")
	otherRunID := common.RunID("run-error-ticket-binding-other")
	ticketID := common.TicketID("ticket-error-ticket-binding")
	stepID := common.StepID("run-error-ticket-binding/manual-gate")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error ticket binding"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureWorkspace(ctx, otherWorkspaceID, "error ticket binding other"); err != nil {
		t.Fatalf("ensure other workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureRun(ctx, otherRunID, workspaceID); err != nil {
		t.Fatalf("ensure other run: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}

	for _, tc := range []struct {
		name        string
		workspaceID common.WorkspaceID
		runID       common.RunID
	}{
		{name: "workspace mismatch", workspaceID: otherWorkspaceID, runID: runID},
		{name: "run mismatch", workspaceID: workspaceID, runID: otherRunID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.EnsureDecisionTicket(ctx, ticketID, tc.workspaceID, tc.runID, stepID, "manual decision")
			if !errors.Is(err, common.ErrConflict) {
				t.Fatalf("expected ErrConflict, got %v", err)
			}
		})
	}
}

func TestEnsureApprovalApprovedReturnsConflictWhenAlreadyRejected(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-approval")
	runID := common.RunID("run-error-approval")
	approvalID := common.ApprovalID("approval-error-approval")
	stepID := common.StepID("run-error-approval/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error approval"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.EnsureApprovalRejected(ctx, approvalID); err != nil {
		t.Fatalf("reject approval: %v", err)
	}

	err := svc.EnsureApprovalApproved(ctx, approvalID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureApprovalApprovedReturnsNilWhenAlreadyApproved(t *testing.T) {
	svc, rt := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-approval-approved-repeat")
	runID := common.RunID("run-error-approval-approved-repeat")
	approvalID := common.ApprovalID("approval-error-approval-approved-repeat")
	stepID := common.StepID("run-error-approval-approved-repeat/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error approval approved repeat"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}
	if err := rt.EnsureApprovalApproved(ctx, approvalID); err != nil {
		t.Fatalf("ensure approval approved: %v", err)
	}

	if err := svc.EnsureApprovalApproved(ctx, approvalID); err != nil {
		t.Fatalf("expected nil for already approved approval, got %v", err)
	}
}

func TestEnsureApprovalReturnsConflictWhenBindingMismatch(t *testing.T) {
	svc, _ := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-approval-binding")
	runID := common.RunID("run-error-approval-binding")
	approvalID := common.ApprovalID("approval-error-approval-binding")
	stepID := common.StepID("run-error-approval-binding/manual-approval")
	otherStepID := common.StepID("run-error-approval-binding/manual-approval-other")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error approval binding"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}

	err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, otherStepID, "manual approval")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureApprovalReturnsConflictWhenWorkspaceOrRunBindingMismatch(t *testing.T) {
	svc, _ := newPhase1TestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-approval-binding-extended")
	otherWorkspaceID := common.WorkspaceID("ws-error-approval-binding-extended-other")
	runID := common.RunID("run-error-approval-binding-extended")
	otherRunID := common.RunID("run-error-approval-binding-extended-other")
	approvalID := common.ApprovalID("approval-error-approval-binding-extended")
	stepID := common.StepID("run-error-approval-binding-extended/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error approval binding extended"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureWorkspace(ctx, otherWorkspaceID, "error approval binding extended other"); err != nil {
		t.Fatalf("ensure other workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureRun(ctx, otherRunID, workspaceID); err != nil {
		t.Fatalf("ensure other run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}

	for _, tc := range []struct {
		name        string
		workspaceID common.WorkspaceID
		runID       common.RunID
	}{
		{name: "workspace mismatch", workspaceID: otherWorkspaceID, runID: runID},
		{name: "run mismatch", workspaceID: workspaceID, runID: otherRunID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.EnsureApproval(ctx, approvalID, tc.workspaceID, tc.runID, stepID, "manual approval")
			if !errors.Is(err, common.ErrConflict) {
				t.Fatalf("expected ErrConflict, got %v", err)
			}
		})
	}
}

func TestEnsureWorkspaceResetsStaleWorkspaceViaModuleService(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")

	workspaceID := common.WorkspaceID("ws-stale-workspace")
	if err := session.Bootstrap.Exports().Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "stale title",
		State:       common.WorkspaceStateArchived,
		ActiveRunID: common.RunID("run-stale"),
	}); err != nil {
		t.Fatalf("seed stale workspace: %v", err)
	}

	svc := NewPhase1Service(session.Runtime)
	if err := svc.EnsureWorkspace(ctx, workspaceID, "phase1 reset workspace"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	item, err := session.Runtime.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace after ensure: %v", err)
	}
	if item.Title != "phase1 reset workspace" {
		t.Fatalf("title = %q, want phase1 reset workspace", item.Title)
	}
	if item.State != common.WorkspaceStateActive {
		t.Fatalf("state = %q, want active", item.State)
	}
	if item.ActiveRunID != "" {
		t.Fatalf("activeRunId = %q, want empty", item.ActiveRunID)
	}
}
