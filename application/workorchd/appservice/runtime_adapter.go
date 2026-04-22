package appservice

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type RuntimeDependencies struct {
	Snapshot RuntimeSnapshotDependencies
	Actions  RuntimeActionDependencies
}

type RuntimeSnapshotDependencies struct {
	Workspaces repository.WorkspaceRepository
	Runs       repository.RunRepository
	Steps      repository.StepRepository
	Tickets    repository.DecisionTicketRepository
	Approvals  repository.ApprovalRepository
}

type RuntimeActionDependencies struct {
	Workspace WorkspaceActionRuntime
	Run       RunActionRuntime
	Decision  DecisionActionRuntime
	Approval  ApprovalActionRuntime
}

type runtimeAdapter struct {
	snapshot RuntimeSnapshotDependencies
	actions  RuntimeActionDependencies
}

func NewRuntimeAdapter(deps RuntimeDependencies) *runtimeAdapter {
	return &runtimeAdapter{
		snapshot: deps.Snapshot,
		actions:  deps.Actions,
	}
}

func (r *runtimeAdapter) GetWorkspaceByID(ctx context.Context, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return r.snapshot.Workspaces.Get(ctx, workspaceID)
}

func (r *runtimeAdapter) EnsureWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	return r.actions.Workspace.EnsureWorkspace(ctx, workspaceID, title)
}

func (r *runtimeAdapter) GetRun(ctx context.Context, runID common.RunID) (*runmodel.Run, error) {
	return r.snapshot.Runs.Get(ctx, runID)
}

func (r *runtimeAdapter) EnsureRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	return r.actions.Run.EnsureRun(ctx, runID, workspaceID)
}

func (r *runtimeAdapter) EnsureRunStarted(ctx context.Context, runID common.RunID) error {
	return r.actions.Run.EnsureRunStarted(ctx, runID)
}

func (r *runtimeAdapter) EnsureRunBlockedOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	return r.actions.Run.EnsureRunBlockedOnDecision(ctx, runID, ticketID)
}

func (r *runtimeAdapter) EnsureRunBlockedOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	return r.actions.Run.EnsureRunBlockedOnApproval(ctx, runID, approvalID)
}

func (r *runtimeAdapter) EnsureRunResumed(ctx context.Context, runID common.RunID) error {
	return r.actions.Run.EnsureRunResumed(ctx, runID)
}

func (r *runtimeAdapter) GetStep(ctx context.Context, stepID common.StepID) (*runmodel.Step, error) {
	return r.snapshot.Steps.Get(ctx, stepID)
}

func (r *runtimeAdapter) EnsureTicket(
	ctx context.Context,
	ticketID common.TicketID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	return r.actions.Decision.EnsureTicket(ctx, ticketID, workspaceID, runID, stepID, reason)
}

func (r *runtimeAdapter) GetTicket(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	return r.snapshot.Tickets.Get(ctx, ticketID)
}

func (r *runtimeAdapter) EnsureResolvedTicket(
	ctx context.Context,
	resolutionID common.ResolutionID,
	ticketID common.TicketID,
	resolutionType string,
	payload map[string]any,
) error {
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
	return r.actions.Approval.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, reason)
}

func (r *runtimeAdapter) GetApproval(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return r.snapshot.Approvals.Get(ctx, approvalID)
}

func (r *runtimeAdapter) EnsureApprovalApproved(ctx context.Context, approvalID common.ApprovalID) error {
	return r.actions.Approval.EnsureApprovalApproved(ctx, approvalID)
}

func (r *runtimeAdapter) EnsureApprovalRejected(ctx context.Context, approvalID common.ApprovalID) error {
	return r.actions.Approval.EnsureApprovalRejected(ctx, approvalID)
}
