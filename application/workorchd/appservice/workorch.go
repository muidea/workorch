package appservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/application/workorchd/observed"
	"github.com/muidea/workorch/application/workorchd/presenter"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	legacycommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type SnapshotReadRuntime interface {
	GetWorkspaceByID(context.Context, common.WorkspaceID) (*workspacemodel.Workspace, error)
	GetRun(context.Context, common.RunID) (*runmodel.Run, error)
	GetStep(context.Context, common.StepID) (*runmodel.Step, error)
	GetTicket(context.Context, common.TicketID) (*decisionmodel.Ticket, error)
	GetResolutionByTicketID(context.Context, common.TicketID) (*decisionmodel.Resolution, error)
	GetApproval(context.Context, common.ApprovalID) (*approvalmodel.Approval, error)
}

type WorkspaceActionRuntime interface {
	EnsureWorkspace(context.Context, common.WorkspaceID, string) error
}

type RunActionRuntime interface {
	EnsureRun(context.Context, common.RunID, common.WorkspaceID) error
	EnsureRunStarted(context.Context, common.RunID) error
	EnsureRunBlockedOnDecision(context.Context, common.RunID, common.TicketID) error
	EnsureRunBlockedOnApproval(context.Context, common.RunID, common.ApprovalID) error
	EnsureRunResumed(context.Context, common.RunID) error
}

type DecisionActionRuntime interface {
	EnsureTicket(context.Context, common.TicketID, common.WorkspaceID, common.RunID, common.StepID, string) error
	EnsureResolvedTicket(context.Context, common.ResolutionID, common.TicketID, string, map[string]any) error
}

type ApprovalActionRuntime interface {
	EnsureApproval(context.Context, common.ApprovalID, common.WorkspaceID, common.RunID, common.StepID, string) error
	EnsureApprovalApproved(context.Context, common.ApprovalID) error
	EnsureApprovalRejected(context.Context, common.ApprovalID) error
}

type ActionRuntime interface {
	WorkspaceActionRuntime
	RunActionRuntime
	DecisionActionRuntime
	ApprovalActionRuntime
}

type Runtime interface {
	SnapshotReadRuntime
	ActionRuntime
}

type WorkorchService struct {
	snapshotReads    SnapshotReadRuntime
	workspaceActions WorkspaceActionRuntime
	runActions       RunActionRuntime
	decisionActions  DecisionActionRuntime
	approvalActions  ApprovalActionRuntime
}

type observedRuntime interface {
	PublishedWorkorchEvents() []events.WorkorchEvent
}

type Slice struct {
	Title    string
	Snapshot *presenter.Snapshot
}

func NewWorkorchService(runtime Runtime) *WorkorchService {
	return &WorkorchService{
		snapshotReads:    runtime,
		workspaceActions: runtime,
		runActions:       runtime,
		decisionActions:  runtime,
		approvalActions:  runtime,
	}
}

func RunAllWorkorch(ctx context.Context, runtime Runtime) ([]Slice, error) {
	if err := validateRequiredDependencies(runtime); err != nil {
		return nil, err
	}
	service := NewWorkorchService(runtime)
	slices := []struct {
		title string
		run   func(context.Context) (*presenter.Snapshot, error)
	}{
		{title: "workorch runnable slice", run: service.RunRunnableSlice},
		{title: "workorch runtime-failed-terminal slice", run: service.RunRuntimeFailedTerminalSlice},
		{title: "workorch multi-step-completed slice", run: service.RunMultiStepCompletedSlice},
		{title: "workorch conditional-branch-completed slice", run: service.RunConditionalBranchCompletedSlice},
		{title: "workorch conditional-branch-review-completed slice", run: service.RunConditionalBranchReviewCompletedSlice},
		{title: "workorch runtime-multi-decision-gate-completed slice", run: service.RunRuntimeMultiDecisionGateCompletedSlice},
		{title: "workorch runtime-multi-decision-gate-second-gate-rejected-terminal slice", run: service.RunRuntimeMultiDecisionGateSecondGateRejectedTerminalSlice},
		{title: "workorch runtime-decision-blocked-resume slice", run: service.RunRuntimeDecisionBlockedResumeSlice},
		{title: "workorch runtime-decision-rejected-terminal slice", run: service.RunRuntimeDecisionRejectedTerminalSlice},
		{title: "workorch runtime-approval-blocked-resume slice", run: service.RunRuntimeApprovalBlockedResumeSlice},
		{title: "workorch runtime-approval-rejected-terminal slice", run: service.RunRuntimeApprovalRejectedTerminalSlice},
		{title: "workorch runtime-mixed-decision-approval-completed slice", run: service.RunRuntimeMixedDecisionApprovalCompletedSlice},
		{title: "workorch runtime-mixed-decision-approval-rejected-terminal slice", run: service.RunRuntimeMixedDecisionApprovalRejectedTerminalSlice},
		{title: "workorch decision-blocked-resume slice", run: service.RunDecisionBlockedResumeSlice},
		{title: "workorch decision-rejected-terminal slice", run: service.RunDecisionRejectedTerminalSlice},
		{title: "workorch approval-blocked-resume slice", run: service.RunApprovalBlockedResumeSlice},
		{title: "workorch approval-rejected-terminal slice", run: service.RunApprovalRejectedTerminalSlice},
	}

	ret := make([]Slice, 0, len(slices))
	for _, item := range slices {
		snapshot, err := item.run(ctx)
		if err != nil {
			return nil, err
		}
		ret = append(ret, Slice{
			Title:    item.title,
			Snapshot: snapshot,
		})
	}
	return ret, nil
}

func (s *WorkorchService) validateSnapshotReads() error {
	if s == nil {
		return common.ErrInvalidArgument
	}
	return validateRequiredDependencies(s.snapshotReads)
}

func (s *WorkorchService) validateWorkspaceActions() error {
	if s == nil {
		return common.ErrInvalidArgument
	}
	return validateRequiredDependencies(s.workspaceActions)
}

func (s *WorkorchService) validateRunActions() error {
	if s == nil {
		return common.ErrInvalidArgument
	}
	return validateRequiredDependencies(s.runActions)
}

func (s *WorkorchService) validateDecisionActions() error {
	if s == nil {
		return common.ErrInvalidArgument
	}
	return validateRequiredDependencies(s.decisionActions)
}

func (s *WorkorchService) validateApprovalActions() error {
	if s == nil {
		return common.ErrInvalidArgument
	}
	return validateRequiredDependencies(s.approvalActions)
}

func (s *WorkorchService) RunRunnableSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-runnable")
	runID := common.RunID("run-workorch-runnable")
	stepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runnable"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		return nil, err
	}

	return s.snapshot(ctx, "runnable", workspaceID, runID, stepID, presenter.BlockingRefs{})
}

func (s *WorkorchService) RunDecisionBlockedResumeSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-blocked-decision")
	runID := common.RunID("run-workorch-blocked-decision")
	manualStepID := common.StepID("run-workorch-blocked-decision/manual-gate")
	ticketID := common.TicketID("ticket-workorch-blocked-decision")
	resumedStepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-decision-blocked-resume"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, manualStepID, "workorch manual decision gate"); err != nil {
		return nil, err
	}
	if err := s.EnsureRunBlockedOnDecision(ctx, runID, ticketID); err != nil {
		return nil, err
	}
	if err := s.EnsureDecisionResolved(ctx, ticketID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunResumed(ctx, runID); err != nil {
		return nil, err
	}

	return s.snapshot(ctx, "decision_blocked_resume", workspaceID, runID, resumedStepID, presenter.BlockingRefs{TicketID: ticketID})
}

func (s *WorkorchService) RunRuntimeFailedTerminalSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-runtime-failed")
	runID := common.RunID("run-workorch-runtime-failed")
	stepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runtime-failed-terminal"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		if strings.Contains(string(runID), legacycommon.RuntimeFailureRunToken) {
			matched, matchErr := s.runtimeFailureTerminalReached(ctx, runID, stepID)
			if matchErr != nil {
				return nil, matchErr
			}
			if matched {
				return s.snapshot(ctx, "runtime_failed_terminal", workspaceID, runID, stepID, presenter.BlockingRefs{})
			}
		}
		return nil, err
	}

	return s.snapshot(ctx, "runtime_failed_terminal", workspaceID, runID, stepID, presenter.BlockingRefs{})
}

func (s *WorkorchService) RunMultiStepCompletedSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-multi-step")
	runID := common.RunID("run-workorch-multi-step")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	step2ID := common.StepID(fmt.Sprintf("%s/step-2", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-multi-step-completed"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "multi_step_completed", workspaceID, runID, step2ID, []common.StepID{step1ID, step2ID}, presenter.BlockingRefs{})
}

func (s *WorkorchService) RunConditionalBranchCompletedSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-conditional-branch")
	runID := common.RunID("run-workorch-conditional-branch")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	branchStepID := common.StepID(fmt.Sprintf("%s/step-fast", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-conditional-branch-completed"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "conditional_branch_completed", workspaceID, runID, branchStepID, []common.StepID{step1ID, branchStepID}, presenter.BlockingRefs{})
}

func (s *WorkorchService) RunConditionalBranchReviewCompletedSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-conditional-branch-review")
	runID := common.RunID("run-workorch-conditional-branch-branch-slow")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	branchStepID := common.StepID(fmt.Sprintf("%s/step-review", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-conditional-branch-review-completed"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "conditional_branch_review_completed", workspaceID, runID, branchStepID, []common.StepID{step1ID, branchStepID}, presenter.BlockingRefs{})
}

func (s *WorkorchService) RunRuntimeMultiDecisionGateCompletedSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-multi-decision-gate")
	runID := common.RunID("run-workorch-multi-decision-gate")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	step2ID := common.StepID(fmt.Sprintf("%s/step-2", runID))
	step3ID := common.StepID(fmt.Sprintf("%s/step-3", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runtime-multi-decision-gate-completed"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		return nil, err
	}

	lastTicketID, err := s.resumeRuntimeDecisionGate(ctx, runID, step1ID)
	if err != nil {
		return nil, err
	}
	if latest, err := s.resumeRuntimeDecisionGate(ctx, runID, step2ID); err != nil {
		return nil, err
	} else if latest != "" {
		lastTicketID = latest
	}

	return s.snapshotWithSteps(ctx, "runtime_multi_decision_gate_completed", workspaceID, runID, step3ID, []common.StepID{step1ID, step2ID, step3ID}, presenter.BlockingRefs{TicketID: lastTicketID})
}

func (s *WorkorchService) RunRuntimeMultiDecisionGateSecondGateRejectedTerminalSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-multi-decision-gate-rejected")
	runID := common.RunID("run-workorch-multi-decision-gate-rejected")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	step2ID := common.StepID(fmt.Sprintf("%s/step-2", runID))
	firstTicketID := common.TicketID(fmt.Sprintf("ticket-%s-gate-1", runID))
	secondTicketID := common.TicketID(fmt.Sprintf("ticket-%s-gate-2", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runtime-multi-decision-gate-second-gate-rejected-terminal"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		matched, matchErr := s.runtimeMultiDecisionSecondGateRejectedTerminalReached(ctx, runID, step1ID, step2ID, firstTicketID, secondTicketID)
		if matchErr != nil {
			return nil, matchErr
		}
		if matched {
			return s.snapshotWithSteps(ctx, "runtime_multi_decision_gate_second_gate_rejected_terminal", workspaceID, runID, "", []common.StepID{step1ID, step2ID}, presenter.BlockingRefs{TicketID: secondTicketID})
		}
		return nil, err
	}

	if _, err := s.resumeRuntimeDecisionGate(ctx, runID, step1ID); err != nil {
		return nil, err
	}
	secondTicketID, err := s.rejectRuntimeDecisionGate(ctx, runID, step2ID)
	if err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "runtime_multi_decision_gate_second_gate_rejected_terminal", workspaceID, runID, "", []common.StepID{step1ID, step2ID}, presenter.BlockingRefs{TicketID: secondTicketID})
}

func (s *WorkorchService) RunRuntimeDecisionBlockedResumeSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-runtime-decision")
	runID := common.RunID("run-workorch-runtime-decision-block")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	step2ID := common.StepID(fmt.Sprintf("%s/step-2", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runtime-decision-blocked-resume"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		return nil, err
	}
	if err := s.validateSnapshotReads(); err != nil {
		return nil, err
	}
	ticketID, err := s.resumeRuntimeDecisionGate(ctx, runID, step1ID)
	if err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "runtime_decision_blocked_resume", workspaceID, runID, step2ID, []common.StepID{step1ID, step2ID}, presenter.BlockingRefs{TicketID: ticketID})
}

func (s *WorkorchService) RunRuntimeDecisionRejectedTerminalSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-runtime-decision-rejected")
	runID := common.RunID("run-workorch-runtime-decision-block-rejected")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	ticketID := common.TicketID(fmt.Sprintf("ticket-%s-runtime-decision", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runtime-decision-rejected-terminal"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		matched, matchErr := s.runtimeDecisionRejectedTerminalReached(ctx, runID, step1ID, ticketID)
		if matchErr != nil {
			return nil, matchErr
		}
		if matched {
			return s.snapshotWithSteps(ctx, "runtime_decision_rejected_terminal", workspaceID, runID, "", []common.StepID{step1ID}, presenter.BlockingRefs{TicketID: ticketID})
		}
		return nil, err
	}

	ticketID, err := s.rejectRuntimeDecisionGate(ctx, runID, step1ID)
	if err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "runtime_decision_rejected_terminal", workspaceID, runID, "", []common.StepID{step1ID}, presenter.BlockingRefs{TicketID: ticketID})
}

func (s *WorkorchService) RunRuntimeApprovalBlockedResumeSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-runtime-approval")
	runID := common.RunID("run-workorch-runtime-approval-block")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	step2ID := common.StepID(fmt.Sprintf("%s/step-2", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runtime-approval-blocked-resume"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		return nil, err
	}
	approvalID, err := s.resumeRuntimeApprovalGate(ctx, runID, step1ID)
	if err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "runtime_approval_blocked_resume", workspaceID, runID, step2ID, []common.StepID{step1ID, step2ID}, presenter.BlockingRefs{ApprovalID: approvalID})
}

func (s *WorkorchService) RunRuntimeApprovalRejectedTerminalSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-runtime-approval-rejected")
	runID := common.RunID("run-workorch-runtime-approval-block-rejected")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	approvalID := common.ApprovalID(fmt.Sprintf("approval-%s-runtime-approval", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runtime-approval-rejected-terminal"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		matched, matchErr := s.runtimeApprovalRejectedTerminalReached(ctx, runID, step1ID, approvalID)
		if matchErr != nil {
			return nil, matchErr
		}
		if matched {
			return s.snapshotWithSteps(ctx, "runtime_approval_rejected_terminal", workspaceID, runID, "", []common.StepID{step1ID}, presenter.BlockingRefs{ApprovalID: approvalID})
		}
		return nil, err
	}
	approvalID, err := s.rejectRuntimeApprovalGate(ctx, runID, step1ID)
	if err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "runtime_approval_rejected_terminal", workspaceID, runID, "", []common.StepID{step1ID}, presenter.BlockingRefs{ApprovalID: approvalID})
}

func (s *WorkorchService) RunRuntimeMixedDecisionApprovalCompletedSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-mixed-decision-approval")
	runID := common.RunID("run-workorch-mixed-decision-approval")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	step2ID := common.StepID(fmt.Sprintf("%s/step-2", runID))
	step3ID := common.StepID(fmt.Sprintf("%s/step-3", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runtime-mixed-decision-approval-completed"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		return nil, err
	}

	ticketID, err := s.resumeRuntimeDecisionGate(ctx, runID, step1ID)
	if err != nil {
		return nil, err
	}
	approvalID, err := s.resumeRuntimeApprovalGate(ctx, runID, step2ID)
	if err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "runtime_mixed_decision_approval_completed", workspaceID, runID, step3ID, []common.StepID{step1ID, step2ID, step3ID}, presenter.BlockingRefs{TicketID: ticketID, ApprovalID: approvalID})
}

func (s *WorkorchService) RunRuntimeMixedDecisionApprovalRejectedTerminalSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-mixed-decision-approval-rejected")
	runID := common.RunID("run-workorch-mixed-decision-approval-rejected")
	step1ID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	step2ID := common.StepID(fmt.Sprintf("%s/step-2", runID))
	ticketID := common.TicketID(fmt.Sprintf("ticket-%s-mixed-decision", runID))
	approvalID := common.ApprovalID(fmt.Sprintf("approval-%s-mixed-approval", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-runtime-mixed-decision-approval-rejected-terminal"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunStarted(ctx, runID); err != nil {
		matched, matchErr := s.runtimeMixedDecisionApprovalRejectedTerminalReached(ctx, runID, step1ID, step2ID, ticketID, approvalID)
		if matchErr != nil {
			return nil, matchErr
		}
		if matched {
			return s.snapshotWithSteps(ctx, "runtime_mixed_decision_approval_rejected_terminal", workspaceID, runID, "", []common.StepID{step1ID, step2ID}, presenter.BlockingRefs{TicketID: ticketID, ApprovalID: approvalID})
		}
		return nil, err
	}

	ticketID, err := s.resumeRuntimeDecisionGate(ctx, runID, step1ID)
	if err != nil {
		return nil, err
	}
	approvalID, err = s.rejectRuntimeApprovalGate(ctx, runID, step2ID)
	if err != nil {
		return nil, err
	}

	return s.snapshotWithSteps(ctx, "runtime_mixed_decision_approval_rejected_terminal", workspaceID, runID, "", []common.StepID{step1ID, step2ID}, presenter.BlockingRefs{TicketID: ticketID, ApprovalID: approvalID})
}

func (s *WorkorchService) runtimeFailureTerminalReached(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
) (bool, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return false, err
	}
	run, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return false, err
	}
	if run.State != common.RunStateFailed {
		return false, nil
	}
	step, err := s.snapshotReads.GetStep(ctx, stepID)
	if err != nil {
		return false, err
	}
	if step.State != common.StepStateFailed {
		return false, nil
	}
	reason, _ := step.Output["reason"].(string)
	return strings.Contains(reason, "legacy runtime execution failed"), nil
}

func (s *WorkorchService) runtimeApprovalRejectedTerminalReached(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
	approvalID common.ApprovalID,
) (bool, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return false, err
	}
	run, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return false, err
	}
	if run.State != common.RunStateFailed {
		return false, nil
	}
	step, err := s.snapshotReads.GetStep(ctx, stepID)
	if err != nil {
		return false, err
	}
	if step.State != common.StepStateCompleted {
		return false, nil
	}
	if runtimeApprovalID(step) != approvalID {
		return false, nil
	}
	approval, err := s.snapshotReads.GetApproval(ctx, approvalID)
	if err != nil {
		return false, err
	}
	return approval.State == common.ApprovalStateRejected, nil
}

func (s *WorkorchService) runtimeDecisionRejectedTerminalReached(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
	ticketID common.TicketID,
) (bool, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return false, err
	}
	run, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return false, err
	}
	if run.State != common.RunStateFailed {
		return false, nil
	}
	step, err := s.snapshotReads.GetStep(ctx, stepID)
	if err != nil {
		return false, err
	}
	if step.State != common.StepStateCompleted || runtimeDecisionTicketID(step) != ticketID {
		return false, nil
	}
	ticket, err := s.snapshotReads.GetTicket(ctx, ticketID)
	if err != nil {
		return false, err
	}
	if ticket.State != common.DecisionTicketStateResolved {
		return false, nil
	}
	resolution, err := s.snapshotReads.GetResolutionByTicketID(ctx, ticketID)
	if err != nil {
		return false, err
	}
	return resolution.Type == decisioncommon.ResolutionTypeRejectPath, nil
}

func (s *WorkorchService) runtimeMultiDecisionSecondGateRejectedTerminalReached(
	ctx context.Context,
	runID common.RunID,
	step1ID common.StepID,
	step2ID common.StepID,
	firstTicketID common.TicketID,
	secondTicketID common.TicketID,
) (bool, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return false, err
	}
	run, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return false, err
	}
	if run.State != common.RunStateFailed {
		return false, nil
	}
	step1, err := s.snapshotReads.GetStep(ctx, step1ID)
	if err != nil {
		return false, err
	}
	if step1.State != common.StepStateCompleted || runtimeDecisionTicketID(step1) != firstTicketID {
		return false, nil
	}
	step2, err := s.snapshotReads.GetStep(ctx, step2ID)
	if err != nil {
		return false, err
	}
	if step2.State != common.StepStateCompleted || runtimeDecisionTicketID(step2) != secondTicketID {
		return false, nil
	}
	firstTicket, err := s.snapshotReads.GetTicket(ctx, firstTicketID)
	if err != nil {
		return false, err
	}
	if firstTicket.State != common.DecisionTicketStateResolved {
		return false, nil
	}
	secondTicket, err := s.snapshotReads.GetTicket(ctx, secondTicketID)
	if err != nil {
		return false, err
	}
	if secondTicket.State != common.DecisionTicketStateResolved {
		return false, nil
	}
	resolution, err := s.snapshotReads.GetResolutionByTicketID(ctx, secondTicketID)
	if err != nil {
		return false, err
	}
	return resolution.Type == decisioncommon.ResolutionTypeRejectPath, nil
}

func (s *WorkorchService) runtimeMixedDecisionApprovalRejectedTerminalReached(
	ctx context.Context,
	runID common.RunID,
	step1ID common.StepID,
	step2ID common.StepID,
	ticketID common.TicketID,
	approvalID common.ApprovalID,
) (bool, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return false, err
	}
	run, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return false, err
	}
	if run.State != common.RunStateFailed {
		return false, nil
	}
	step1, err := s.snapshotReads.GetStep(ctx, step1ID)
	if err != nil {
		return false, err
	}
	if step1.State != common.StepStateCompleted || runtimeDecisionTicketID(step1) != ticketID {
		return false, nil
	}
	step2, err := s.snapshotReads.GetStep(ctx, step2ID)
	if err != nil {
		return false, err
	}
	if step2.State != common.StepStateCompleted || runtimeApprovalID(step2) != approvalID {
		return false, nil
	}
	ticket, err := s.snapshotReads.GetTicket(ctx, ticketID)
	if err != nil {
		return false, err
	}
	if ticket.State != common.DecisionTicketStateResolved {
		return false, nil
	}
	approval, err := s.snapshotReads.GetApproval(ctx, approvalID)
	if err != nil {
		return false, err
	}
	return approval.State == common.ApprovalStateRejected, nil
}

func (s *WorkorchService) RunApprovalBlockedResumeSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-blocked-approval")
	runID := common.RunID("run-workorch-blocked-approval")
	manualStepID := common.StepID("run-workorch-blocked-approval/manual-approval")
	approvalID := common.ApprovalID("approval-workorch-blocked-approval")
	resumedStepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-approval-blocked-resume"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureApproval(ctx, approvalID, workspaceID, runID, manualStepID, "workorch manual approval gate"); err != nil {
		return nil, err
	}
	if err := s.EnsureRunBlockedOnApproval(ctx, runID, approvalID); err != nil {
		return nil, err
	}
	if err := s.EnsureApprovalApproved(ctx, approvalID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunResumed(ctx, runID); err != nil {
		return nil, err
	}

	return s.snapshot(ctx, "approval_blocked_resume", workspaceID, runID, resumedStepID, presenter.BlockingRefs{ApprovalID: approvalID})
}

func (s *WorkorchService) RunDecisionRejectedTerminalSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-rejected-decision")
	runID := common.RunID("run-workorch-rejected-decision")
	manualStepID := common.StepID("run-workorch-rejected-decision/manual-gate")
	ticketID := common.TicketID("ticket-workorch-rejected-decision")

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-decision-rejected-terminal"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, manualStepID, "workorch manual decision rejection gate"); err != nil {
		return nil, err
	}
	if err := s.EnsureRunBlockedOnDecision(ctx, runID, ticketID); err != nil {
		return nil, err
	}
	if err := s.EnsureDecisionRejected(ctx, ticketID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunResumed(ctx, runID); err != nil {
		return nil, err
	}

	return s.snapshot(ctx, "decision_rejected_terminal", workspaceID, runID, "", presenter.BlockingRefs{TicketID: ticketID})
}

func (s *WorkorchService) RunApprovalRejectedTerminalSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-workorch-rejected-approval")
	runID := common.RunID("run-workorch-rejected-approval")
	manualStepID := common.StepID("run-workorch-rejected-approval/manual-approval")
	approvalID := common.ApprovalID("approval-workorch-rejected-approval")

	if err := s.EnsureWorkspace(ctx, workspaceID, "workorch-approval-rejected-terminal"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureApproval(ctx, approvalID, workspaceID, runID, manualStepID, "workorch manual approval rejection gate"); err != nil {
		return nil, err
	}
	if err := s.EnsureRunBlockedOnApproval(ctx, runID, approvalID); err != nil {
		return nil, err
	}
	if err := s.EnsureApprovalRejected(ctx, approvalID); err != nil {
		return nil, err
	}
	if err := s.EnsureRunResumed(ctx, runID); err != nil {
		return nil, err
	}

	return s.snapshot(ctx, "approval_rejected_terminal", workspaceID, runID, "", presenter.BlockingRefs{ApprovalID: approvalID})
}

func (s *WorkorchService) EnsureWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	if err := s.validateWorkspaceActions(); err != nil {
		return err
	}
	return s.workspaceActions.EnsureWorkspace(ctx, workspaceID, title)
}

func (s *WorkorchService) EnsureRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if err := s.validateRunActions(); err != nil {
		return err
	}
	return s.runActions.EnsureRun(ctx, runID, workspaceID)
}

func (s *WorkorchService) EnsureRunStarted(ctx context.Context, runID common.RunID) error {
	if err := s.validateRunActions(); err != nil {
		return err
	}
	return s.runActions.EnsureRunStarted(ctx, runID)
}

func (s *WorkorchService) EnsureDecisionTicket(
	ctx context.Context,
	ticketID common.TicketID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if err := s.validateDecisionActions(); err != nil {
		return err
	}
	return s.decisionActions.EnsureTicket(ctx, ticketID, workspaceID, runID, stepID, reason)
}

func (s *WorkorchService) EnsureRunBlockedOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if err := s.validateRunActions(); err != nil {
		return err
	}
	return s.runActions.EnsureRunBlockedOnDecision(ctx, runID, ticketID)
}

func (s *WorkorchService) EnsureDecisionResolved(ctx context.Context, ticketID common.TicketID) error {
	return s.ensureDecisionResolution(
		ctx,
		common.ResolutionID("resolution-workorch-blocked-decision"),
		ticketID,
		decisioncommon.ResolutionTypeApprovePath,
		map[string]any{"operator": "workorch-demo"},
	)
}

func (s *WorkorchService) EnsureDecisionRejected(ctx context.Context, ticketID common.TicketID) error {
	return s.ensureDecisionResolution(
		ctx,
		common.ResolutionID("resolution-workorch-rejected-decision"),
		ticketID,
		decisioncommon.ResolutionTypeRejectPath,
		map[string]any{"operator": "workorch-demo", "outcome": "reject"},
	)
}

func (s *WorkorchService) ensureDecisionResolution(
	ctx context.Context,
	resolutionID common.ResolutionID,
	ticketID common.TicketID,
	resolutionType string,
	payload map[string]any,
) error {
	if err := s.validateDecisionActions(); err != nil {
		return err
	}
	return s.decisionActions.EnsureResolvedTicket(
		ctx,
		resolutionID,
		ticketID,
		resolutionType,
		payload,
	)
}

func (s *WorkorchService) EnsureApproval(
	ctx context.Context,
	approvalID common.ApprovalID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if err := s.validateApprovalActions(); err != nil {
		return err
	}
	return s.approvalActions.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, reason)
}

func (s *WorkorchService) EnsureRunBlockedOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if err := s.validateRunActions(); err != nil {
		return err
	}
	return s.runActions.EnsureRunBlockedOnApproval(ctx, runID, approvalID)
}

func (s *WorkorchService) EnsureApprovalApproved(ctx context.Context, approvalID common.ApprovalID) error {
	if err := s.validateApprovalActions(); err != nil {
		return err
	}
	return s.approvalActions.EnsureApprovalApproved(ctx, approvalID)
}

func (s *WorkorchService) EnsureApprovalRejected(ctx context.Context, approvalID common.ApprovalID) error {
	if err := s.validateApprovalActions(); err != nil {
		return err
	}
	return s.approvalActions.EnsureApprovalRejected(ctx, approvalID)
}

func (s *WorkorchService) EnsureRunResumed(ctx context.Context, runID common.RunID) error {
	if err := s.validateRunActions(); err != nil {
		return err
	}
	return s.runActions.EnsureRunResumed(ctx, runID)
}

func (s *WorkorchService) snapshot(
	ctx context.Context,
	slice string,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	refs presenter.BlockingRefs,
) (*presenter.Snapshot, error) {
	return s.snapshotWithSteps(ctx, slice, workspaceID, runID, stepID, nil, refs)
}

func runtimeDecisionTicketID(step *runmodel.Step) common.TicketID {
	if step == nil || step.Output == nil {
		return ""
	}
	ticketID, _ := step.Output["blockDecisionTicketId"].(string)
	return common.TicketID(ticketID)
}

func runtimeApprovalID(step *runmodel.Step) common.ApprovalID {
	if step == nil || step.Output == nil {
		return ""
	}
	approvalID, _ := step.Output["blockApprovalId"].(string)
	return common.ApprovalID(approvalID)
}

func (s *WorkorchService) resumeRuntimeDecisionGate(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
) (common.TicketID, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return "", err
	}
	step, err := s.snapshotReads.GetStep(ctx, stepID)
	if err != nil {
		return "", err
	}
	ticketID := runtimeDecisionTicketID(step)

	run, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	if ticketID == "" {
		ticketID = run.BlockingTicketID
	}
	if run.State != common.RunStateBlockedOnDecision {
		return ticketID, nil
	}
	if ticketID == "" {
		return "", common.ErrConflict
	}
	if err := s.EnsureDecisionResolved(ctx, ticketID); err != nil {
		return "", err
	}
	if err := s.EnsureRunResumed(ctx, runID); err != nil {
		return "", err
	}
	return ticketID, nil
}

func (s *WorkorchService) rejectRuntimeDecisionGate(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
) (common.TicketID, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return "", err
	}
	step, err := s.snapshotReads.GetStep(ctx, stepID)
	if err != nil {
		return "", err
	}
	ticketID := runtimeDecisionTicketID(step)

	run, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	if ticketID == "" {
		ticketID = run.BlockingTicketID
	}
	if run.State != common.RunStateBlockedOnDecision {
		return ticketID, nil
	}
	if ticketID == "" {
		return "", common.ErrConflict
	}
	if err := s.EnsureDecisionRejected(ctx, ticketID); err != nil {
		return "", err
	}
	if err := s.EnsureRunResumed(ctx, runID); err != nil {
		return "", err
	}
	return ticketID, nil
}

func (s *WorkorchService) resumeRuntimeApprovalGate(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
) (common.ApprovalID, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return "", err
	}
	step, err := s.snapshotReads.GetStep(ctx, stepID)
	if err != nil {
		return "", err
	}
	approvalID := runtimeApprovalID(step)

	run, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	if approvalID == "" {
		approvalID = run.BlockingApprovalID
	}
	if run.State != common.RunStateBlockedOnApproval {
		return approvalID, nil
	}
	if approvalID == "" {
		return "", common.ErrConflict
	}
	if err := s.EnsureApprovalApproved(ctx, approvalID); err != nil {
		return "", err
	}
	if err := s.EnsureRunResumed(ctx, runID); err != nil {
		return "", err
	}
	return approvalID, nil
}

func (s *WorkorchService) rejectRuntimeApprovalGate(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
) (common.ApprovalID, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return "", err
	}
	step, err := s.snapshotReads.GetStep(ctx, stepID)
	if err != nil {
		return "", err
	}
	approvalID := runtimeApprovalID(step)

	run, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	if approvalID == "" {
		approvalID = run.BlockingApprovalID
	}
	if run.State != common.RunStateBlockedOnApproval {
		return approvalID, nil
	}
	if approvalID == "" {
		return "", common.ErrConflict
	}
	if err := s.EnsureApprovalRejected(ctx, approvalID); err != nil {
		return "", err
	}
	if err := s.EnsureRunResumed(ctx, runID); err != nil {
		return "", err
	}
	return approvalID, nil
}

func (s *WorkorchService) snapshotWithSteps(
	ctx context.Context,
	slice string,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	stepIDs []common.StepID,
	refs presenter.BlockingRefs,
) (*presenter.Snapshot, error) {
	if err := s.validateSnapshotReads(); err != nil {
		return nil, err
	}
	ws, err := s.snapshotReads.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	r, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	var step *runmodel.Step
	steps, err := s.snapshotSteps(ctx, stepID, stepIDs)
	if err != nil {
		return nil, err
	}
	if stepID != "" {
		for _, item := range steps {
			if item != nil && item.ID == stepID {
				step = item
				break
			}
		}
	}

	var ticket *decisionmodel.Ticket
	var resolution *decisionmodel.Resolution
	if refs.TicketID != "" {
		ticket, err = s.snapshotReads.GetTicket(ctx, refs.TicketID)
		if err != nil {
			return nil, err
		}
		if ticket != nil && ticket.State == common.DecisionTicketStateResolved {
			resolution, err = s.snapshotReads.GetResolutionByTicketID(ctx, ticket.ID)
			if err != nil {
				return nil, err
			}
		}
	}

	var approval *approvalmodel.Approval
	if refs.ApprovalID != "" {
		approval, err = s.snapshotReads.GetApproval(ctx, refs.ApprovalID)
		if err != nil {
			return nil, err
		}
	}

	var published []events.WorkorchEvent
	if source, ok := s.snapshotReads.(observedRuntime); ok {
		published = source.PublishedWorkorchEvents()
	}

	return presenter.BuildSnapshot(slice, ws, r, step, steps, ticket, approval, observed.BuildWorkorchSnapshot(ws, r, steps, ticket, resolution, approval, published)), nil
}

func (s *WorkorchService) snapshotSteps(ctx context.Context, primary common.StepID, extra []common.StepID) ([]*runmodel.Step, error) {
	ordered := make([]common.StepID, 0, 1+len(extra))
	seen := map[common.StepID]struct{}{}
	for _, id := range extra {
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}
	if primary != "" {
		if _, exists := seen[primary]; !exists {
			ordered = append(ordered, primary)
		}
	}
	if len(ordered) == 0 {
		return nil, nil
	}

	steps := make([]*runmodel.Step, 0, len(ordered))
	for _, id := range ordered {
		step, err := s.snapshotReads.GetStep(ctx, id)
		if err != nil {
			if err == common.ErrNotFound {
				continue
			}
			return nil, err
		}
		steps = append(steps, step)
	}
	if len(steps) == 0 {
		return nil, nil
	}
	return steps, nil
}
