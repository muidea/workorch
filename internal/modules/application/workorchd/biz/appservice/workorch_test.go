package appservice

import (
	"context"
	"errors"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type noopWorkspaceActions struct{}

func (noopWorkspaceActions) EnsureWorkspace(context.Context, common.WorkspaceID, string) error {
	return nil
}

type noopRunActions struct{}

func (noopRunActions) EnsureRun(context.Context, common.RunID, common.WorkspaceID) error {
	return nil
}

func (noopRunActions) EnsureRunStarted(context.Context, common.RunID) error {
	return nil
}

func (noopRunActions) EnsureRunBlockedOnDecision(context.Context, common.RunID, common.TicketID) error {
	return nil
}

func (noopRunActions) EnsureRunBlockedOnApproval(context.Context, common.RunID, common.ApprovalID) error {
	return nil
}

func (noopRunActions) EnsureRunResumed(context.Context, common.RunID) error {
	return nil
}

func TestRunAllWorkorchReturnsInvalidArgumentWhenRuntimeMissing(t *testing.T) {
	if _, err := RunAllWorkorch(context.Background(), nil); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestListWorkorchSlicesReturnsStableCatalog(t *testing.T) {
	slices := ListWorkorchSlices()
	if len(slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(slices))
	}
	if slices[0].ID != "workorch-runnable" || slices[0].Title != "workorch runnable slice" {
		t.Fatalf("first slice = %+v, want workorch-runnable/workorch runnable slice", slices[0])
	}
	if slices[len(slices)-1].ID != "workorch-approval-rejected-terminal" {
		t.Fatalf("last slice id = %q, want workorch-approval-rejected-terminal", slices[len(slices)-1].ID)
	}
}

func TestRunWorkorchSliceReturnsInvalidArgumentWhenRuntimeMissing(t *testing.T) {
	if _, err := RunWorkorchSlice(context.Background(), nil, "workorch-runnable"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestRunWorkorchSliceReturnsNotFoundForUnknownSlice(t *testing.T) {
	_, runtime := newWorkorchTestService(t)
	if _, err := RunWorkorchSlice(context.Background(), runtime, "workorch-missing"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestRunWorkorchSliceReturnsSingleRequestedSlice(t *testing.T) {
	_, runtime := newWorkorchTestService(t)
	item, err := RunWorkorchSlice(context.Background(), runtime, "workorch-runnable")
	if err != nil {
		t.Fatalf("RunWorkorchSlice: %v", err)
	}
	if item.ID != "workorch-runnable" || item.Title != "workorch runnable slice" {
		t.Fatalf("slice = %+v, want workorch-runnable/workorch runnable slice", item)
	}
	if item.Snapshot == nil || item.Snapshot.Slice != "runnable" {
		t.Fatalf("snapshot = %+v, want runnable snapshot", item.Snapshot)
	}
}

func TestReadWorkorchSliceSnapshotReturnsNotFoundBeforeExecution(t *testing.T) {
	_, runtime := newWorkorchTestService(t)
	if _, err := ReadWorkorchSliceSnapshot(context.Background(), runtime, "workorch-runnable"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestSummarizeWorkorchSliceStatusReturnsExpectedSummary(t *testing.T) {
	_, runtime := newWorkorchTestService(t)
	if _, err := RunWorkorchSlice(context.Background(), runtime, "workorch-runnable"); err != nil {
		t.Fatalf("RunWorkorchSlice: %v", err)
	}

	status, err := SummarizeWorkorchSliceStatus(context.Background(), runtime, "workorch-runnable")
	if err != nil {
		t.Fatalf("SummarizeWorkorchSliceStatus: %v", err)
	}
	if status.ID != "workorch-runnable" || status.Title != "workorch runnable slice" || status.Slice != "runnable" {
		t.Fatalf("status = %+v, want runnable summary", status)
	}
	if status.WorkspaceID != "ws-workorch-runnable" || status.RunID != "run-workorch-runnable" {
		t.Fatalf("status ids = %+v, want runnable ids", status)
	}
	if status.RunState != "completed" || status.StepState != "completed" {
		t.Fatalf("status states = %+v, want completed/completed", status)
	}
}

func TestListAvailableWorkorchSliceStatusesReturnsOnlyExistingStates(t *testing.T) {
	_, runtime := newWorkorchTestService(t)

	statuses, err := ListAvailableWorkorchSliceStatuses(context.Background(), runtime)
	if err != nil {
		t.Fatalf("ListAvailableWorkorchSliceStatuses before run: %v", err)
	}
	if len(statuses) != 0 {
		t.Fatalf("len(statuses) = %d, want 0 before run", len(statuses))
	}

	if _, err := RunWorkorchSlice(context.Background(), runtime, "workorch-runnable"); err != nil {
		t.Fatalf("RunWorkorchSlice: %v", err)
	}

	statuses, err = ListAvailableWorkorchSliceStatuses(context.Background(), runtime)
	if err != nil {
		t.Fatalf("ListAvailableWorkorchSliceStatuses after run: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("len(statuses) = %d, want 1 after single run", len(statuses))
	}
	if statuses[0].ID != "workorch-runnable" || statuses[0].RunState != "completed" {
		t.Fatalf("status = %+v, want completed runnable summary", statuses[0])
	}
}

func TestListAvailableWorkorchSliceSnapshotsReturnsOnlyExistingStates(t *testing.T) {
	_, runtime := newWorkorchTestService(t)

	slices, err := ListAvailableWorkorchSliceSnapshots(context.Background(), runtime)
	if err != nil {
		t.Fatalf("ListAvailableWorkorchSliceSnapshots before run: %v", err)
	}
	if len(slices) != 0 {
		t.Fatalf("len(slices) = %d, want 0 before run", len(slices))
	}

	if _, err := RunWorkorchSlice(context.Background(), runtime, "workorch-runnable"); err != nil {
		t.Fatalf("RunWorkorchSlice: %v", err)
	}

	slices, err = ListAvailableWorkorchSliceSnapshots(context.Background(), runtime)
	if err != nil {
		t.Fatalf("ListAvailableWorkorchSliceSnapshots after run: %v", err)
	}
	if len(slices) != 1 {
		t.Fatalf("len(slices) = %d, want 1 after single run", len(slices))
	}
	if slices[0].ID != "workorch-runnable" || slices[0].Snapshot == nil || slices[0].Snapshot.Slice != "runnable" {
		t.Fatalf("slice = %+v, want runnable snapshot", slices[0])
	}
}

func TestWorkorchServiceActionMethodsReturnInvalidArgumentWhenDependenciesMissing(t *testing.T) {
	svc := NewWorkorchService(nil)

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "ensure workspace", run: func() error { return svc.EnsureWorkspace(context.Background(), "ws", "title") }},
		{name: "ensure run", run: func() error { return svc.EnsureRun(context.Background(), "run", "ws") }},
		{name: "ensure decision ticket", run: func() error {
			return svc.EnsureDecisionTicket(context.Background(), "ticket", "ws", "run", "run/manual-gate", "decision")
		}},
		{name: "ensure decision resolved", run: func() error { return svc.EnsureDecisionResolved(context.Background(), "ticket") }},
		{name: "ensure decision rejected", run: func() error { return svc.EnsureDecisionRejected(context.Background(), "ticket") }},
		{name: "ensure approval", run: func() error {
			return svc.EnsureApproval(context.Background(), "approval", "ws", "run", "run/manual-approval", "approval")
		}},
		{name: "ensure approval approved", run: func() error { return svc.EnsureApprovalApproved(context.Background(), "approval") }},
		{name: "ensure approval rejected", run: func() error { return svc.EnsureApprovalRejected(context.Background(), "approval") }},
		{name: "ensure run resumed", run: func() error { return svc.EnsureRunResumed(context.Background(), "run") }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
		})
	}
}

func TestRunRunnableSliceReturnsInvalidArgumentWhenSnapshotReadsMissing(t *testing.T) {
	svc := &WorkorchService{
		workspaceActions: noopWorkspaceActions{},
		runActions:       noopRunActions{},
	}

	if _, err := svc.RunRunnableSlice(context.Background()); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestEnsureRunStartedReturnsConflictWhenRunBlocked(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
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
	svc, rt := newWorkorchTestService(t)
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
	svc := NewWorkorchService(session.Runtime)

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

func TestRunRuntimeFailedTerminalSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunRuntimeFailedTerminalSlice(ctx)
	if err != nil {
		t.Fatalf("RunRuntimeFailedTerminalSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil || snapshot.Step == nil {
		t.Fatalf("snapshot = %+v, want failed run and failed step", snapshot)
	}
	if snapshot.Slice != "runtime_failed_terminal" {
		t.Fatalf("snapshot slice = %s, want runtime_failed_terminal", snapshot.Slice)
	}
	if snapshot.Run.State != "failed" {
		t.Fatalf("snapshot run state = %s, want failed", snapshot.Run.State)
	}
	if snapshot.Step.State != "failed" {
		t.Fatalf("snapshot step state = %s, want failed", snapshot.Step.State)
	}
	if snapshot.Step.Output["reason"] == nil {
		t.Fatalf("snapshot step output missing reason: %+v", snapshot.Step.Output)
	}
}

func TestRunMultiStepCompletedSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunMultiStepCompletedSlice(ctx)
	if err != nil {
		t.Fatalf("RunMultiStepCompletedSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil || snapshot.Step == nil {
		t.Fatalf("snapshot = %+v, want completed run and final step", snapshot)
	}
	if snapshot.Slice != "multi_step_completed" {
		t.Fatalf("snapshot slice = %s, want multi_step_completed", snapshot.Slice)
	}
	if snapshot.Run.State != "completed" {
		t.Fatalf("snapshot run state = %s, want completed", snapshot.Run.State)
	}
	if snapshot.Step.ID != "run-workorch-multi-step/step-2" || snapshot.Step.State != "completed" {
		t.Fatalf("snapshot step = %+v, want completed step-2", snapshot.Step)
	}
	if len(snapshot.Steps) != 2 {
		t.Fatalf("snapshot steps len = %d, want 2", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-multi-step/step-1" || snapshot.Steps[1].ID != "run-workorch-multi-step/step-2" {
		t.Fatalf("snapshot steps = %+v, want step-1 then step-2", snapshot.Steps)
	}
}

func TestRunConditionalBranchCompletedSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunConditionalBranchCompletedSlice(ctx)
	if err != nil {
		t.Fatalf("RunConditionalBranchCompletedSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil || snapshot.Step == nil {
		t.Fatalf("snapshot = %+v, want completed run and branch step", snapshot)
	}
	if snapshot.Slice != "conditional_branch_completed" {
		t.Fatalf("snapshot slice = %s, want conditional_branch_completed", snapshot.Slice)
	}
	if snapshot.Run.State != "completed" {
		t.Fatalf("snapshot run state = %s, want completed", snapshot.Run.State)
	}
	if snapshot.Step.ID != "run-workorch-conditional-branch/step-fast" || snapshot.Step.State != "completed" {
		t.Fatalf("snapshot step = %+v, want completed branch step", snapshot.Step)
	}
	if len(snapshot.Steps) != 2 {
		t.Fatalf("snapshot steps len = %d, want 2", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-conditional-branch/step-1" || snapshot.Steps[1].ID != "run-workorch-conditional-branch/step-fast" {
		t.Fatalf("snapshot steps = %+v, want step-1 then step-fast", snapshot.Steps)
	}
	if snapshot.Steps[0].Output["branch"] != "fast-track" {
		t.Fatalf("branch output = %v, want fast-track", snapshot.Steps[0].Output["branch"])
	}
}

func TestRunConditionalBranchReviewCompletedSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunConditionalBranchReviewCompletedSlice(ctx)
	if err != nil {
		t.Fatalf("RunConditionalBranchReviewCompletedSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil || snapshot.Step == nil {
		t.Fatalf("snapshot = %+v, want completed run and review branch step", snapshot)
	}
	if snapshot.Slice != "conditional_branch_review_completed" {
		t.Fatalf("snapshot slice = %s, want conditional_branch_review_completed", snapshot.Slice)
	}
	if snapshot.Run.State != "completed" {
		t.Fatalf("snapshot run state = %s, want completed", snapshot.Run.State)
	}
	if snapshot.Step.ID != "run-workorch-conditional-branch-branch-slow/step-review" || snapshot.Step.State != "completed" {
		t.Fatalf("snapshot step = %+v, want completed review branch step", snapshot.Step)
	}
	if len(snapshot.Steps) != 2 {
		t.Fatalf("snapshot steps len = %d, want 2", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-conditional-branch-branch-slow/step-1" || snapshot.Steps[1].ID != "run-workorch-conditional-branch-branch-slow/step-review" {
		t.Fatalf("snapshot steps = %+v, want step-1 then step-review", snapshot.Steps)
	}
	if snapshot.Steps[0].Output["branch"] != "review-path" {
		t.Fatalf("branch output = %v, want review-path", snapshot.Steps[0].Output["branch"])
	}
}

func TestRunRuntimeMultiDecisionGateCompletedSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunRuntimeMultiDecisionGateCompletedSlice(ctx)
	if err != nil {
		t.Fatalf("RunRuntimeMultiDecisionGateCompletedSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil || snapshot.Step == nil {
		t.Fatalf("snapshot = %+v, want completed run and final step", snapshot)
	}
	if snapshot.Slice != "runtime_multi_decision_gate_completed" {
		t.Fatalf("snapshot slice = %s, want runtime_multi_decision_gate_completed", snapshot.Slice)
	}
	if snapshot.Run.State != "completed" {
		t.Fatalf("snapshot run state = %s, want completed", snapshot.Run.State)
	}
	if snapshot.Step.ID != "run-workorch-multi-decision-gate/step-3" || snapshot.Step.State != "completed" {
		t.Fatalf("snapshot step = %+v, want completed step-3", snapshot.Step)
	}
	if len(snapshot.Steps) != 3 {
		t.Fatalf("snapshot steps len = %d, want 3", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-multi-decision-gate/step-1" || snapshot.Steps[1].ID != "run-workorch-multi-decision-gate/step-2" || snapshot.Steps[2].ID != "run-workorch-multi-decision-gate/step-3" {
		t.Fatalf("snapshot steps = %+v, want step-1 then step-2 then step-3", snapshot.Steps)
	}
	if snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-gate-1" {
		t.Fatalf("step1 blockDecisionTicketId = %v, want ticket-run-workorch-multi-decision-gate-gate-1", snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if snapshot.Steps[1].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-gate-2" {
		t.Fatalf("step2 blockDecisionTicketId = %v, want ticket-run-workorch-multi-decision-gate-gate-2", snapshot.Steps[1].Output["blockDecisionTicketId"])
	}
	if snapshot.Blocking == nil || snapshot.Blocking.DecisionTicket == nil {
		t.Fatalf("snapshot blocking = %+v, want decision ticket", snapshot.Blocking)
	}
	if snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-multi-decision-gate-gate-2" {
		t.Fatalf("decision ticket id = %s, want ticket-run-workorch-multi-decision-gate-gate-2", snapshot.Blocking.DecisionTicket.ID)
	}
	if snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("decision ticket state = %s, want resolved", snapshot.Blocking.DecisionTicket.State)
	}
}

func TestRunRuntimeMultiDecisionGateSecondGateRejectedTerminalSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunRuntimeMultiDecisionGateSecondGateRejectedTerminalSlice(ctx)
	if err != nil {
		t.Fatalf("RunRuntimeMultiDecisionGateSecondGateRejectedTerminalSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil {
		t.Fatalf("snapshot = %+v, want failed run", snapshot)
	}
	if snapshot.Slice != "runtime_multi_decision_gate_second_gate_rejected_terminal" {
		t.Fatalf("snapshot slice = %s, want runtime_multi_decision_gate_second_gate_rejected_terminal", snapshot.Slice)
	}
	if snapshot.Run.State != "failed" {
		t.Fatalf("snapshot run state = %s, want failed", snapshot.Run.State)
	}
	if snapshot.Step != nil {
		t.Fatalf("snapshot step = %+v, want nil", snapshot.Step)
	}
	if len(snapshot.Steps) != 2 {
		t.Fatalf("snapshot steps len = %d, want 2", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-multi-decision-gate-rejected/step-1" || snapshot.Steps[1].ID != "run-workorch-multi-decision-gate-rejected/step-2" {
		t.Fatalf("snapshot steps = %+v, want completed step-1 then step-2", snapshot.Steps)
	}
	if snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-rejected-gate-1" {
		t.Fatalf("step1 blockDecisionTicketId = %v, want ticket-run-workorch-multi-decision-gate-rejected-gate-1", snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if snapshot.Steps[1].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-rejected-gate-2" {
		t.Fatalf("step2 blockDecisionTicketId = %v, want ticket-run-workorch-multi-decision-gate-rejected-gate-2", snapshot.Steps[1].Output["blockDecisionTicketId"])
	}
	if snapshot.Blocking == nil || snapshot.Blocking.DecisionTicket == nil {
		t.Fatalf("snapshot blocking = %+v, want decision ticket", snapshot.Blocking)
	}
	if snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-multi-decision-gate-rejected-gate-2" {
		t.Fatalf("decision ticket id = %s, want ticket-run-workorch-multi-decision-gate-rejected-gate-2", snapshot.Blocking.DecisionTicket.ID)
	}
	if snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("decision ticket state = %s, want resolved", snapshot.Blocking.DecisionTicket.State)
	}
}

func TestRunRuntimeDecisionBlockedResumeSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunRuntimeDecisionBlockedResumeSlice(ctx)
	if err != nil {
		t.Fatalf("RunRuntimeDecisionBlockedResumeSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil || snapshot.Step == nil {
		t.Fatalf("snapshot = %+v, want completed run and final step", snapshot)
	}
	if snapshot.Slice != "runtime_decision_blocked_resume" {
		t.Fatalf("snapshot slice = %s, want runtime_decision_blocked_resume", snapshot.Slice)
	}
	if snapshot.Run.State != "completed" {
		t.Fatalf("snapshot run state = %s, want completed", snapshot.Run.State)
	}
	if snapshot.Step.ID != "run-workorch-runtime-decision-block/step-2" || snapshot.Step.State != "completed" {
		t.Fatalf("snapshot step = %+v, want completed step-2", snapshot.Step)
	}
	if len(snapshot.Steps) != 2 {
		t.Fatalf("snapshot steps len = %d, want 2", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-runtime-decision-block/step-1" || snapshot.Steps[1].ID != "run-workorch-runtime-decision-block/step-2" {
		t.Fatalf("snapshot steps = %+v, want step-1 then step-2", snapshot.Steps)
	}
	if snapshot.Blocking == nil || snapshot.Blocking.DecisionTicket == nil {
		t.Fatalf("snapshot blocking = %+v, want decision ticket", snapshot.Blocking)
	}
	if snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("decision ticket state = %s, want resolved", snapshot.Blocking.DecisionTicket.State)
	}
}

func TestRunRuntimeDecisionRejectedTerminalSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunRuntimeDecisionRejectedTerminalSlice(ctx)
	if err != nil {
		t.Fatalf("RunRuntimeDecisionRejectedTerminalSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil {
		t.Fatalf("snapshot = %+v, want failed run", snapshot)
	}
	if snapshot.Slice != "runtime_decision_rejected_terminal" {
		t.Fatalf("snapshot slice = %s, want runtime_decision_rejected_terminal", snapshot.Slice)
	}
	if snapshot.Run.State != "failed" {
		t.Fatalf("snapshot run state = %s, want failed", snapshot.Run.State)
	}
	if snapshot.Step != nil {
		t.Fatalf("snapshot step = %+v, want nil", snapshot.Step)
	}
	if len(snapshot.Steps) != 1 {
		t.Fatalf("snapshot steps len = %d, want 1", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-runtime-decision-block-rejected/step-1" || snapshot.Steps[0].State != "completed" {
		t.Fatalf("snapshot steps = %+v, want completed step-1 only", snapshot.Steps)
	}
	if snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-runtime-decision-block-rejected-runtime-decision" {
		t.Fatalf("step1 blockDecisionTicketId = %v, want ticket-run-workorch-runtime-decision-block-rejected-runtime-decision", snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if snapshot.Blocking == nil || snapshot.Blocking.DecisionTicket == nil {
		t.Fatalf("snapshot blocking = %+v, want decision ticket", snapshot.Blocking)
	}
	if snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-runtime-decision-block-rejected-runtime-decision" {
		t.Fatalf("decision ticket id = %s, want ticket-run-workorch-runtime-decision-block-rejected-runtime-decision", snapshot.Blocking.DecisionTicket.ID)
	}
	if snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("decision ticket state = %s, want resolved", snapshot.Blocking.DecisionTicket.State)
	}
}

func TestRunRuntimeApprovalBlockedResumeSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunRuntimeApprovalBlockedResumeSlice(ctx)
	if err != nil {
		t.Fatalf("RunRuntimeApprovalBlockedResumeSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil || snapshot.Step == nil {
		t.Fatalf("snapshot = %+v, want completed run and final step", snapshot)
	}
	if snapshot.Slice != "runtime_approval_blocked_resume" {
		t.Fatalf("snapshot slice = %s, want runtime_approval_blocked_resume", snapshot.Slice)
	}
	if snapshot.Run.State != "completed" {
		t.Fatalf("snapshot run state = %s, want completed", snapshot.Run.State)
	}
	if snapshot.Step.ID != "run-workorch-runtime-approval-block/step-2" || snapshot.Step.State != "completed" {
		t.Fatalf("snapshot step = %+v, want completed step-2", snapshot.Step)
	}
	if len(snapshot.Steps) != 2 {
		t.Fatalf("snapshot steps len = %d, want 2", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-runtime-approval-block/step-1" || snapshot.Steps[1].ID != "run-workorch-runtime-approval-block/step-2" {
		t.Fatalf("snapshot steps = %+v, want step-1 then step-2", snapshot.Steps)
	}
	if snapshot.Steps[0].Output["blockApprovalId"] != "approval-run-workorch-runtime-approval-block-runtime-approval" {
		t.Fatalf("step1 blockApprovalId = %v, want approval-run-workorch-runtime-approval-block-runtime-approval", snapshot.Steps[0].Output["blockApprovalId"])
	}
	if snapshot.Blocking == nil || snapshot.Blocking.Approval == nil {
		t.Fatalf("snapshot blocking = %+v, want approval", snapshot.Blocking)
	}
	if snapshot.Blocking.Approval.ID != "approval-run-workorch-runtime-approval-block-runtime-approval" {
		t.Fatalf("approval id = %s, want approval-run-workorch-runtime-approval-block-runtime-approval", snapshot.Blocking.Approval.ID)
	}
	if snapshot.Blocking.Approval.State != "approved" {
		t.Fatalf("approval state = %s, want approved", snapshot.Blocking.Approval.State)
	}
}

func TestRunRuntimeApprovalRejectedTerminalSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunRuntimeApprovalRejectedTerminalSlice(ctx)
	if err != nil {
		t.Fatalf("RunRuntimeApprovalRejectedTerminalSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil {
		t.Fatalf("snapshot = %+v, want failed run", snapshot)
	}
	if snapshot.Slice != "runtime_approval_rejected_terminal" {
		t.Fatalf("snapshot slice = %s, want runtime_approval_rejected_terminal", snapshot.Slice)
	}
	if snapshot.Run.State != "failed" {
		t.Fatalf("snapshot run state = %s, want failed", snapshot.Run.State)
	}
	if snapshot.Step != nil {
		t.Fatalf("snapshot step = %+v, want nil", snapshot.Step)
	}
	if len(snapshot.Steps) != 1 {
		t.Fatalf("snapshot steps len = %d, want 1", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-runtime-approval-block-rejected/step-1" || snapshot.Steps[0].State != "completed" {
		t.Fatalf("snapshot steps = %+v, want completed step-1 only", snapshot.Steps)
	}
	if snapshot.Steps[0].Output["blockApprovalId"] != "approval-run-workorch-runtime-approval-block-rejected-runtime-approval" {
		t.Fatalf("step1 blockApprovalId = %v, want approval-run-workorch-runtime-approval-block-rejected-runtime-approval", snapshot.Steps[0].Output["blockApprovalId"])
	}
	if snapshot.Blocking == nil || snapshot.Blocking.Approval == nil {
		t.Fatalf("snapshot blocking = %+v, want approval", snapshot.Blocking)
	}
	if snapshot.Blocking.Approval.ID != "approval-run-workorch-runtime-approval-block-rejected-runtime-approval" {
		t.Fatalf("approval id = %s, want approval-run-workorch-runtime-approval-block-rejected-runtime-approval", snapshot.Blocking.Approval.ID)
	}
	if snapshot.Blocking.Approval.State != "rejected" {
		t.Fatalf("approval state = %s, want rejected", snapshot.Blocking.Approval.State)
	}
}

func TestRunRuntimeMixedDecisionApprovalCompletedSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunRuntimeMixedDecisionApprovalCompletedSlice(ctx)
	if err != nil {
		t.Fatalf("RunRuntimeMixedDecisionApprovalCompletedSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil || snapshot.Step == nil {
		t.Fatalf("snapshot = %+v, want completed run and final step", snapshot)
	}
	if snapshot.Slice != "runtime_mixed_decision_approval_completed" {
		t.Fatalf("snapshot slice = %s, want runtime_mixed_decision_approval_completed", snapshot.Slice)
	}
	if snapshot.Run.State != "completed" {
		t.Fatalf("snapshot run state = %s, want completed", snapshot.Run.State)
	}
	if snapshot.Step.ID != "run-workorch-mixed-decision-approval/step-3" || snapshot.Step.State != "completed" {
		t.Fatalf("snapshot step = %+v, want completed step-3", snapshot.Step)
	}
	if len(snapshot.Steps) != 3 {
		t.Fatalf("snapshot steps len = %d, want 3", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-mixed-decision-approval/step-1" || snapshot.Steps[1].ID != "run-workorch-mixed-decision-approval/step-2" || snapshot.Steps[2].ID != "run-workorch-mixed-decision-approval/step-3" {
		t.Fatalf("snapshot steps = %+v, want step-1 then step-2 then step-3", snapshot.Steps)
	}
	if snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-mixed-decision-approval-mixed-decision" {
		t.Fatalf("step1 blockDecisionTicketId = %v, want ticket-run-workorch-mixed-decision-approval-mixed-decision", snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if snapshot.Steps[1].Output["blockApprovalId"] != "approval-run-workorch-mixed-decision-approval-mixed-approval" {
		t.Fatalf("step2 blockApprovalId = %v, want approval-run-workorch-mixed-decision-approval-mixed-approval", snapshot.Steps[1].Output["blockApprovalId"])
	}
	if snapshot.Blocking == nil || snapshot.Blocking.DecisionTicket == nil || snapshot.Blocking.Approval == nil {
		t.Fatalf("snapshot blocking = %+v, want decision ticket and approval", snapshot.Blocking)
	}
	if snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-mixed-decision-approval-mixed-decision" || snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("decision ticket = %+v, want resolved mixed decision ticket", snapshot.Blocking.DecisionTicket)
	}
	if snapshot.Blocking.Approval.ID != "approval-run-workorch-mixed-decision-approval-mixed-approval" || snapshot.Blocking.Approval.State != "approved" {
		t.Fatalf("approval = %+v, want approved mixed approval", snapshot.Blocking.Approval)
	}
}

func TestRunRuntimeMixedDecisionApprovalRejectedTerminalSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunRuntimeMixedDecisionApprovalRejectedTerminalSlice(ctx)
	if err != nil {
		t.Fatalf("RunRuntimeMixedDecisionApprovalRejectedTerminalSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil {
		t.Fatalf("snapshot = %+v, want failed run", snapshot)
	}
	if snapshot.Slice != "runtime_mixed_decision_approval_rejected_terminal" {
		t.Fatalf("snapshot slice = %s, want runtime_mixed_decision_approval_rejected_terminal", snapshot.Slice)
	}
	if snapshot.Run.State != "failed" {
		t.Fatalf("snapshot run state = %s, want failed", snapshot.Run.State)
	}
	if snapshot.Step != nil {
		t.Fatalf("snapshot step = %+v, want nil", snapshot.Step)
	}
	if len(snapshot.Steps) != 2 {
		t.Fatalf("snapshot steps len = %d, want 2", len(snapshot.Steps))
	}
	if snapshot.Steps[0].ID != "run-workorch-mixed-decision-approval-rejected/step-1" || snapshot.Steps[0].State != "completed" {
		t.Fatalf("snapshot step-1 = %+v, want completed step-1", snapshot.Steps[0])
	}
	if snapshot.Steps[1].ID != "run-workorch-mixed-decision-approval-rejected/step-2" || snapshot.Steps[1].State != "completed" {
		t.Fatalf("snapshot step-2 = %+v, want completed step-2", snapshot.Steps[1])
	}
	if snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-mixed-decision-approval-rejected-mixed-decision" {
		t.Fatalf("step1 blockDecisionTicketId = %v, want ticket-run-workorch-mixed-decision-approval-rejected-mixed-decision", snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if snapshot.Steps[1].Output["blockApprovalId"] != "approval-run-workorch-mixed-decision-approval-rejected-mixed-approval" {
		t.Fatalf("step2 blockApprovalId = %v, want approval-run-workorch-mixed-decision-approval-rejected-mixed-approval", snapshot.Steps[1].Output["blockApprovalId"])
	}
	if snapshot.Blocking == nil || snapshot.Blocking.DecisionTicket == nil || snapshot.Blocking.Approval == nil {
		t.Fatalf("snapshot blocking = %+v, want decision ticket and approval", snapshot.Blocking)
	}
	if snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-mixed-decision-approval-rejected-mixed-decision" || snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("decision ticket = %+v, want resolved mixed decision ticket", snapshot.Blocking.DecisionTicket)
	}
	if snapshot.Blocking.Approval.ID != "approval-run-workorch-mixed-decision-approval-rejected-mixed-approval" || snapshot.Blocking.Approval.State != "rejected" {
		t.Fatalf("approval = %+v, want rejected mixed approval", snapshot.Blocking.Approval)
	}
}

func TestEnsureRunResumedReturnsConflictWhenRunNotBlocked(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
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
	svc, rt := newWorkorchTestService(t)
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

func TestEnsureRunResumedReturnsBlockedWhenApprovalStillPending(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-error-pending-approval")
	runID := common.RunID("run-error-pending-approval")
	approvalID := common.ApprovalID("approval-error-pending-approval")
	stepID := common.StepID("run-error-pending-approval/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "error pending approval"); err != nil {
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

	err := svc.EnsureRunResumed(ctx, runID)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
}

func TestEnsureRunResumedReturnsNilWhenRunAlreadyFailedByRejectedApproval(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
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

func TestEnsureRunResumedReturnsNilWhenRunAlreadyFailedByRejectedDecision(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-failed-decision-run")
	runID := common.RunID("run-failed-decision-run")
	ticketID := common.TicketID("ticket-failed-decision-run")
	stepID := common.StepID("run-failed-decision-run/manual-gate")
	runtimeStepID := common.StepID("run-failed-decision-run/step-1")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "failed decision run"); err != nil {
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
	if err := rt.EnsureResolvedTicket(ctx, common.ResolutionID("resolution-failed-decision-run"), ticketID, decisioncommon.ResolutionTypeRejectCurrentPath, map[string]any{"operator": "workorch"}); err != nil {
		t.Fatalf("reject decision ticket: %v", err)
	}
	if err := rt.EnsureRunResumed(ctx, runID); err != nil {
		t.Fatalf("resume rejected decision run: %v", err)
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
		t.Fatalf("expected no runtime step after rejected decision, got %v", err)
	}
}

func TestEnsureRunResumedReturnsNilWhenRunAlreadyCompleted(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
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
	svc, rt := newWorkorchTestService(t)
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
	if err := rt.EnsureResolvedTicket(ctx, common.ResolutionID("resolution-blocked-decision-repeat"), ticketID, "provide_missing_info", map[string]any{"operator": "workorch"}); err != nil {
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
	svc, rt := newWorkorchTestService(t)
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

func TestEnsureRunBlockedOnDecisionReturnsConflictWhenBlockedByDifferentReference(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-blocked-decision-conflict")
	runID := common.RunID("run-blocked-decision-conflict")
	firstTicketID := common.TicketID("ticket-blocked-decision-conflict-1")
	secondTicketID := common.TicketID("ticket-blocked-decision-conflict-2")
	stepID := common.StepID("run-blocked-decision-conflict/manual-gate")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "blocked decision conflict"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, firstTicketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure first decision ticket: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, secondTicketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure second decision ticket: %v", err)
	}
	if err := rt.EnsureRunBlockedOnDecision(ctx, runID, firstTicketID); err != nil {
		t.Fatalf("ensure blocked on decision: %v", err)
	}

	err := svc.EnsureRunBlockedOnDecision(ctx, runID, secondTicketID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	runItem, err := rt.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID != firstTicketID || runItem.BlockingApprovalID != "" {
		t.Fatalf("run mutated after decision blocking conflict: %+v", runItem)
	}
}

func TestEnsureRunBlockedOnApprovalReturnsConflictWhenBlockedByDifferentReference(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-blocked-approval-conflict")
	runID := common.RunID("run-blocked-approval-conflict")
	firstApprovalID := common.ApprovalID("approval-blocked-approval-conflict-1")
	secondApprovalID := common.ApprovalID("approval-blocked-approval-conflict-2")
	stepID := common.StepID("run-blocked-approval-conflict/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "blocked approval conflict"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, firstApprovalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure first approval: %v", err)
	}
	if err := svc.EnsureApproval(ctx, secondApprovalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure second approval: %v", err)
	}
	if err := rt.EnsureRunBlockedOnApproval(ctx, runID, firstApprovalID); err != nil {
		t.Fatalf("ensure blocked on approval: %v", err)
	}

	err := svc.EnsureRunBlockedOnApproval(ctx, runID, secondApprovalID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	runItem, err := rt.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnApproval || runItem.BlockingApprovalID != firstApprovalID || runItem.BlockingTicketID != "" {
		t.Fatalf("run mutated after approval blocking conflict: %+v", runItem)
	}
}

func TestEnsureDecisionResolvedReturnsNilWhenTicketAlreadyResolved(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
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
	if err := rt.EnsureResolvedTicket(ctx, common.ResolutionID("resolution-workorch-blocked-decision"), ticketID, "provide_missing_info", map[string]any{"operator": "workorch"}); err != nil {
		t.Fatalf("ensure resolved ticket: %v", err)
	}

	if err := svc.EnsureDecisionResolved(ctx, ticketID); err != nil {
		t.Fatalf("expected nil for already resolved ticket, got %v", err)
	}
}

func TestEnsureDecisionResolvedReturnsConflictWhenResolutionMismatch(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-decision-resolved-mismatch")
	runID := common.RunID("run-decision-resolved-mismatch")
	ticketID := common.TicketID("ticket-decision-resolved-mismatch")
	stepID := common.StepID("run-decision-resolved-mismatch/manual-gate")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "decision resolved mismatch"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}
	if err := rt.EnsureResolvedTicket(ctx, common.ResolutionID("resolution-decision-resolved-other"), ticketID, "reject_current_path", map[string]any{"operator": "other"}); err != nil {
		t.Fatalf("ensure resolved ticket: %v", err)
	}

	err := svc.EnsureDecisionResolved(ctx, ticketID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureDecisionResolvedReturnsNotFoundWhenTicketMissing(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	err := svc.EnsureDecisionResolved(ctx, common.TicketID("ticket-missing"))
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestEnsureDecisionRejectedReturnsNilWhenTicketAlreadyRejected(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-decision-rejected")
	runID := common.RunID("run-decision-rejected")
	ticketID := common.TicketID("ticket-decision-rejected")
	stepID := common.StepID("run-decision-rejected/manual-gate")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "decision rejected"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}
	if err := rt.EnsureResolvedTicket(ctx, common.ResolutionID("resolution-workorch-rejected-decision"), ticketID, decisioncommon.ResolutionTypeRejectCurrentPath, map[string]any{"operator": "workorch", "outcome": "reject"}); err != nil {
		t.Fatalf("ensure rejected ticket: %v", err)
	}

	if err := svc.EnsureDecisionRejected(ctx, ticketID); err != nil {
		t.Fatalf("expected nil for already rejected ticket, got %v", err)
	}
}

func TestRunDecisionRejectedTerminalSlice(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	snapshot, err := svc.RunDecisionRejectedTerminalSlice(ctx)
	if err != nil {
		t.Fatalf("RunDecisionRejectedTerminalSlice: %v", err)
	}
	if snapshot == nil || snapshot.Run == nil {
		t.Fatalf("snapshot run = %+v, want failed run snapshot", snapshot)
	}
	if snapshot.Slice != "decision_rejected_terminal" {
		t.Fatalf("snapshot slice = %s, want decision_rejected_terminal", snapshot.Slice)
	}
	if snapshot.Run.State != "failed" {
		t.Fatalf("snapshot run state = %s, want failed", snapshot.Run.State)
	}
	if snapshot.Step != nil {
		t.Fatalf("snapshot step = %+v, want nil", snapshot.Step)
	}
	if snapshot.Blocking == nil || snapshot.Blocking.DecisionTicket == nil {
		t.Fatalf("snapshot blocking = %+v, want decision ticket", snapshot.Blocking)
	}
	if snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("ticket state = %s, want resolved", snapshot.Blocking.DecisionTicket.State)
	}
}

func TestEnsureRunReturnsConflictWhenWorkspaceMismatch(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
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
	svc, rt := newWorkorchTestService(t)
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

func TestEnsureApprovalRejectedReturnsNotFoundWhenApprovalMissing(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	err := svc.EnsureApprovalRejected(ctx, common.ApprovalID("approval-missing"))
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestEnsureApprovalRejectedReturnsNilWhenAlreadyRejected(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
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
	svc, _ := newWorkorchTestService(t)
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

func TestEnsureDecisionTicketReturnsConflictWhenReasonMismatch(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-decision-ticket-reason-mismatch")
	runID := common.RunID("run-decision-ticket-reason-mismatch")
	ticketID := common.TicketID("ticket-decision-ticket-reason-mismatch")
	stepID := common.StepID("run-decision-ticket-reason-mismatch/manual-gate")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "decision ticket reason mismatch"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, stepID, "manual decision"); err != nil {
		t.Fatalf("ensure decision ticket: %v", err)
	}

	err := svc.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, stepID, "other decision")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureDecisionTicketReturnsConflictWhenWorkspaceOrRunBindingMismatch(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
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
	svc, rt := newWorkorchTestService(t)
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

func TestEnsureApprovalApprovedReturnsNotFoundWhenApprovalMissing(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	err := svc.EnsureApprovalApproved(ctx, common.ApprovalID("approval-missing"))
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestEnsureApprovalApprovedReturnsNilWhenAlreadyApproved(t *testing.T) {
	svc, rt := newWorkorchTestService(t)
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
	svc, _ := newWorkorchTestService(t)
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

func TestEnsureApprovalReturnsConflictWhenReasonMismatch(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-approval-reason-mismatch")
	runID := common.RunID("run-approval-reason-mismatch")
	approvalID := common.ApprovalID("approval-approval-reason-mismatch")
	stepID := common.StepID("run-approval-reason-mismatch/manual-approval")

	if err := svc.EnsureWorkspace(ctx, workspaceID, "approval reason mismatch"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := svc.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "manual approval"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}

	err := svc.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "other approval")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestEnsureApprovalReturnsConflictWhenWorkspaceOrRunBindingMismatch(t *testing.T) {
	svc, _ := newWorkorchTestService(t)
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

	svc := NewWorkorchService(session.Runtime)
	if err := svc.EnsureWorkspace(ctx, workspaceID, "workorch reset workspace"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	item, err := session.Runtime.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace after ensure: %v", err)
	}
	if item.Title != "workorch reset workspace" {
		t.Fatalf("title = %q, want workorch reset workspace", item.Title)
	}
	if item.State != common.WorkspaceStateActive {
		t.Fatalf("state = %q, want active", item.State)
	}
	if item.ActiveRunID != "" {
		t.Fatalf("activeRunId = %q, want empty", item.ActiveRunID)
	}
}
