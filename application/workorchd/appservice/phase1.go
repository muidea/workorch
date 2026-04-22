package appservice

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/application/workorchd/observed"
	"github.com/muidea/workorch/application/workorchd/presenter"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
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

type Phase1Service struct {
	snapshotReads    SnapshotReadRuntime
	workspaceActions WorkspaceActionRuntime
	runActions       RunActionRuntime
	decisionActions  DecisionActionRuntime
	approvalActions  ApprovalActionRuntime
}

type observedRuntime interface {
	PublishedPhase1Events() []events.Phase1Event
}

type Slice struct {
	Title    string
	Snapshot *presenter.Snapshot
}

func NewPhase1Service(runtime Runtime) *Phase1Service {
	return &Phase1Service{
		snapshotReads:    runtime,
		workspaceActions: runtime,
		runActions:       runtime,
		decisionActions:  runtime,
		approvalActions:  runtime,
	}
}

func RunAllPhase1(ctx context.Context, runtime Runtime) ([]Slice, error) {
	service := NewPhase1Service(runtime)
	slices := []struct {
		title string
		run   func(context.Context) (*presenter.Snapshot, error)
	}{
		{title: "phase1 runnable slice", run: service.RunRunnableSlice},
		{title: "phase1 decision-blocked-resume slice", run: service.RunDecisionBlockedResumeSlice},
		{title: "phase1 approval-blocked-resume slice", run: service.RunApprovalBlockedResumeSlice},
		{title: "phase1 approval-rejected-terminal slice", run: service.RunApprovalRejectedTerminalSlice},
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

func (s *Phase1Service) RunRunnableSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-phase1-runnable")
	runID := common.RunID("run-phase1-runnable")
	stepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "phase1-runnable"); err != nil {
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

func (s *Phase1Service) RunDecisionBlockedResumeSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-phase1-blocked-decision")
	runID := common.RunID("run-phase1-blocked-decision")
	manualStepID := common.StepID("run-phase1-blocked-decision/manual-gate")
	ticketID := common.TicketID("ticket-phase1-blocked-decision")
	resumedStepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "phase1-decision-blocked-resume"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureDecisionTicket(ctx, ticketID, workspaceID, runID, manualStepID, "phase1 manual decision gate"); err != nil {
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

func (s *Phase1Service) RunApprovalBlockedResumeSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-phase1-blocked-approval")
	runID := common.RunID("run-phase1-blocked-approval")
	manualStepID := common.StepID("run-phase1-blocked-approval/manual-approval")
	approvalID := common.ApprovalID("approval-phase1-blocked-approval")
	resumedStepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := s.EnsureWorkspace(ctx, workspaceID, "phase1-approval-blocked-resume"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureApproval(ctx, approvalID, workspaceID, runID, manualStepID, "phase1 manual approval gate"); err != nil {
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

func (s *Phase1Service) RunApprovalRejectedTerminalSlice(ctx context.Context) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-phase1-rejected-approval")
	runID := common.RunID("run-phase1-rejected-approval")
	manualStepID := common.StepID("run-phase1-rejected-approval/manual-approval")
	approvalID := common.ApprovalID("approval-phase1-rejected-approval")

	if err := s.EnsureWorkspace(ctx, workspaceID, "phase1-approval-rejected-terminal"); err != nil {
		return nil, err
	}
	if err := s.EnsureRun(ctx, runID, workspaceID); err != nil {
		return nil, err
	}
	if err := s.EnsureApproval(ctx, approvalID, workspaceID, runID, manualStepID, "phase1 manual approval rejection gate"); err != nil {
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

func (s *Phase1Service) EnsureWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	return s.workspaceActions.EnsureWorkspace(ctx, workspaceID, title)
}

func (s *Phase1Service) EnsureRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	return s.runActions.EnsureRun(ctx, runID, workspaceID)
}

func (s *Phase1Service) EnsureRunStarted(ctx context.Context, runID common.RunID) error {
	return s.runActions.EnsureRunStarted(ctx, runID)
}

func (s *Phase1Service) EnsureDecisionTicket(
	ctx context.Context,
	ticketID common.TicketID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	return s.decisionActions.EnsureTicket(ctx, ticketID, workspaceID, runID, stepID, reason)
}

func (s *Phase1Service) EnsureRunBlockedOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	return s.runActions.EnsureRunBlockedOnDecision(ctx, runID, ticketID)
}

func (s *Phase1Service) EnsureDecisionResolved(ctx context.Context, ticketID common.TicketID) error {
	return s.decisionActions.EnsureResolvedTicket(
		ctx,
		common.ResolutionID("resolution-phase1-blocked-decision"),
		ticketID,
		"approve_path",
		map[string]any{"operator": "phase1-demo"},
	)
}

func (s *Phase1Service) EnsureApproval(
	ctx context.Context,
	approvalID common.ApprovalID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	return s.approvalActions.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, reason)
}

func (s *Phase1Service) EnsureRunBlockedOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	return s.runActions.EnsureRunBlockedOnApproval(ctx, runID, approvalID)
}

func (s *Phase1Service) EnsureApprovalApproved(ctx context.Context, approvalID common.ApprovalID) error {
	return s.approvalActions.EnsureApprovalApproved(ctx, approvalID)
}

func (s *Phase1Service) EnsureApprovalRejected(ctx context.Context, approvalID common.ApprovalID) error {
	return s.approvalActions.EnsureApprovalRejected(ctx, approvalID)
}

func (s *Phase1Service) EnsureRunResumed(ctx context.Context, runID common.RunID) error {
	return s.runActions.EnsureRunResumed(ctx, runID)
}

func (s *Phase1Service) snapshot(
	ctx context.Context,
	slice string,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	refs presenter.BlockingRefs,
) (*presenter.Snapshot, error) {
	ws, err := s.snapshotReads.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	r, err := s.snapshotReads.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	var step *runmodel.Step
	if stepID != "" {
		step, err = s.snapshotReads.GetStep(ctx, stepID)
		if err != nil && err != common.ErrNotFound {
			return nil, err
		}
	}

	var ticket *decisionmodel.Ticket
	if refs.TicketID != "" {
		ticket, err = s.snapshotReads.GetTicket(ctx, refs.TicketID)
		if err != nil {
			return nil, err
		}
	}

	var approval *approvalmodel.Approval
	if refs.ApprovalID != "" {
		approval, err = s.snapshotReads.GetApproval(ctx, refs.ApprovalID)
		if err != nil {
			return nil, err
		}
	}

	var published []events.Phase1Event
	if source, ok := s.snapshotReads.(observedRuntime); ok {
		published = source.PublishedPhase1Events()
	}

	return presenter.BuildSnapshot(slice, ws, r, step, ticket, approval, observed.BuildPhase1Snapshot(ws, r, step, ticket, approval, published)), nil
}
