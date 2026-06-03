package appservice

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type RuntimeDependencies struct {
	Snapshot SnapshotReadRuntime
	Actions  RuntimeActionDependencies
}

type RuntimeActionDependencies struct {
	Workspace WorkspaceActionRuntime
	Run       RunActionRuntime
	Decision  DecisionActionRuntime
	Approval  ApprovalActionRuntime
}

type runtimeAdapter struct {
	snapshot SnapshotReadRuntime
	actions  RuntimeActionDependencies
}

func NewRuntimeAdapter(deps RuntimeDependencies) *runtimeAdapter {
	return &runtimeAdapter{
		snapshot: deps.Snapshot,
		actions:  deps.Actions,
	}
}

func (r *runtimeAdapter) GetWorkspaceByID(ctx context.Context, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetWorkspaceByID(ctx, workspaceID)
}

func (r *runtimeAdapter) EnsureWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Workspace); err != nil {
		return err
	}
	return r.actions.Workspace.EnsureWorkspace(ctx, workspaceID, title)
}

func (r *runtimeAdapter) GetRun(ctx context.Context, runID common.RunID) (*runmodel.Run, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetRun(ctx, runID)
}

func (r *runtimeAdapter) EnsureRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRun(ctx, runID, workspaceID)
}

func (r *runtimeAdapter) EnsureRunStarted(ctx context.Context, runID common.RunID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRunStarted(ctx, runID)
}

func (r *runtimeAdapter) EnsureRunBlockedOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRunBlockedOnDecision(ctx, runID, ticketID)
}

func (r *runtimeAdapter) EnsureRunBlockedOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRunBlockedOnApproval(ctx, runID, approvalID)
}

func (r *runtimeAdapter) EnsureRunResumed(ctx context.Context, runID common.RunID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Run); err != nil {
		return err
	}
	return r.actions.Run.EnsureRunResumed(ctx, runID)
}

func (r *runtimeAdapter) GetStep(ctx context.Context, stepID common.StepID) (*runmodel.Step, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetStep(ctx, stepID)
}

func (r *runtimeAdapter) EnsureTicket(
	ctx context.Context,
	ticketID common.TicketID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Decision); err != nil {
		return err
	}
	return r.actions.Decision.EnsureTicket(ctx, ticketID, workspaceID, runID, stepID, reason)
}

func (r *runtimeAdapter) GetTicket(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetTicket(ctx, ticketID)
}

func (r *runtimeAdapter) GetResolutionByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetResolutionByTicketID(ctx, ticketID)
}

func (r *runtimeAdapter) EnsureResolvedTicket(
	ctx context.Context,
	resolutionID common.ResolutionID,
	ticketID common.TicketID,
	resolutionType string,
	payload map[string]any,
) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Decision); err != nil {
		return err
	}
	return r.actions.Decision.EnsureResolvedTicket(ctx, resolutionID, ticketID, resolutionType, payload)
}

func (r *runtimeAdapter) EnsureApproval(
	ctx context.Context,
	approvalID common.ApprovalID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Approval); err != nil {
		return err
	}
	return r.actions.Approval.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, reason)
}

func (r *runtimeAdapter) GetApproval(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetApproval(ctx, approvalID)
}

func (r *runtimeAdapter) EnsureApprovalApproved(ctx context.Context, approvalID common.ApprovalID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Approval); err != nil {
		return err
	}
	return r.actions.Approval.EnsureApprovalApproved(ctx, approvalID)
}

func (r *runtimeAdapter) EnsureApprovalRejected(ctx context.Context, approvalID common.ApprovalID) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Approval); err != nil {
		return err
	}
	return r.actions.Approval.EnsureApprovalRejected(ctx, approvalID)
}

type eventHubSnapshotReadPort struct {
	hub events.Hub
}

func newEventHubSnapshotReadPort(hub events.Hub) SnapshotReadRuntime {
	if hub == nil {
		return nil
	}
	return eventHubSnapshotReadPort{hub: hub}
}

func (p eventHubSnapshotReadPort) GetWorkspaceByID(ctx context.Context, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return readWorkspaceByOwnerCommand(ctx, p.hub, workspaceID)
}

func (p eventHubSnapshotReadPort) GetRun(ctx context.Context, runID common.RunID) (*runmodel.Run, error) {
	return readRunByOwnerCommand(ctx, p.hub, runID)
}

func (p eventHubSnapshotReadPort) GetStep(ctx context.Context, stepID common.StepID) (*runmodel.Step, error) {
	return readStepByOwnerCommand(ctx, p.hub, stepID)
}

func (p eventHubSnapshotReadPort) GetTicket(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	return readDecisionTicketByOwnerCommand(ctx, p.hub, ticketID)
}

func (p eventHubSnapshotReadPort) GetResolutionByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	return readDecisionResolutionByTicketOwnerCommand(ctx, p.hub, ticketID)
}

func (p eventHubSnapshotReadPort) GetApproval(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return readApprovalByOwnerCommand(ctx, p.hub, approvalID)
}
