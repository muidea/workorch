package bootstrap

import (
	"context"

	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Runtime struct {
	workspaceRepo repository.WorkspaceRepository
	runRepo       repository.RunRepository
	stepRepo      repository.StepRepository
	ticketRepo    repository.DecisionTicketRepository
	approvalRepo  repository.ApprovalRepository

	workspaceManager *workspacemodule.GoalDoDManager
	decisionService  *decisionmodule.Service
	approvalService  *approvalmodule.Service
	runService       *runmodule.Service
}

func (r *Runtime) GetWorkspaceByID(ctx context.Context, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return r.workspaceRepo.Get(ctx, workspaceID)
}

func (r *Runtime) UpdateWorkspace(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.workspaceRepo.Update(ctx, item)
}

func (r *Runtime) CreateWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	return r.workspaceManager.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
		WorkspaceID: workspaceID,
		Title:       title,
	})
}

func (r *Runtime) GetRun(ctx context.Context, runID common.RunID) (*runmodel.Run, error) {
	return r.runRepo.Get(ctx, runID)
}

func (r *Runtime) CreateRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	return r.runService.CreateRun(ctx, runmodule.CreateRunCommand{
		RunID:       runID,
		WorkspaceID: workspaceID,
	})
}

func (r *Runtime) StartRun(ctx context.Context, runID common.RunID) error {
	return r.runService.StartRun(ctx, runmodule.StartRunCommand{RunID: runID})
}

func (r *Runtime) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	return r.runService.BlockOnDecision(ctx, runmodule.BlockOnDecisionCommand{
		RunID:    runID,
		TicketID: ticketID,
	})
}

func (r *Runtime) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	return r.runService.BlockOnApproval(ctx, runmodule.BlockOnApprovalCommand{
		RunID:      runID,
		ApprovalID: approvalID,
	})
}

func (r *Runtime) ResumeRun(ctx context.Context, runID common.RunID) error {
	return r.runService.ResumeRun(ctx, runmodule.ResumeRunCommand{RunID: runID})
}

func (r *Runtime) GetStep(ctx context.Context, stepID common.StepID) (*runmodel.Step, error) {
	return r.stepRepo.Get(ctx, stepID)
}

func (r *Runtime) GetTicket(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	return r.ticketRepo.Get(ctx, ticketID)
}

func (r *Runtime) CreateTicket(
	ctx context.Context,
	ticketID common.TicketID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	return r.decisionService.CreateTicket(ctx, decisionmodule.CreateTicketCommand{
		TicketID:    ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      reason,
	})
}

func (r *Runtime) ResolveTicket(
	ctx context.Context,
	resolutionID common.ResolutionID,
	ticketID common.TicketID,
	resolutionType string,
	payload map[string]any,
) error {
	return r.decisionService.ResolveTicket(ctx, decisionmodule.ResolveTicketCommand{
		ResolutionID: resolutionID,
		TicketID:     ticketID,
		Type:         resolutionType,
		Payload:      payload,
	})
}

func (r *Runtime) GetApproval(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return r.approvalRepo.Get(ctx, approvalID)
}

func (r *Runtime) CreateApproval(
	ctx context.Context,
	approvalID common.ApprovalID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	return r.approvalService.CreateApproval(ctx, approvalmodule.CreateApprovalCommand{
		ApprovalID:  approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      reason,
	})
}

func (r *Runtime) ApproveApproval(ctx context.Context, approvalID common.ApprovalID) error {
	return r.approvalService.Approve(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: approvalID})
}

func (r *Runtime) RejectApproval(ctx context.Context, approvalID common.ApprovalID) error {
	return r.approvalService.Reject(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: approvalID})
}
