package appservice

import (
	"context"

	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceServiceActions struct {
	service workspacemodule.Service
}

func newWorkspaceActionPort(service workspacemodule.Service) WorkspaceActionRuntime {
	return workspaceServiceActions{service: service}
}

func (p workspaceServiceActions) EnsureWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureWorkspace(ctx, workspacemodule.EnsureWorkspaceCommand{
		WorkspaceID:          workspaceID,
		Title:                title,
		DeliverableDirectory: "workspaces/" + string(workspaceID),
	})
}

type runServiceActions struct {
	service runmodule.Service
}

func newRunActionPort(service runmodule.Service) RunActionRuntime {
	return runServiceActions{service: service}
}

func (p runServiceActions) EnsureRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureRun(ctx, runmodule.CreateRunCommand{
		RunID:       runID,
		WorkspaceID: workspaceID,
	})
}

func (p runServiceActions) EnsureRunStarted(ctx context.Context, runID common.RunID) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureStarted(ctx, runmodule.StartRunCommand{RunID: runID})
}

func (p runServiceActions) EnsureRunBlockedOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureBlockedOnDecision(ctx, runmodule.BlockOnDecisionCommand{
		RunID:    runID,
		TicketID: ticketID,
	})
}

func (p runServiceActions) EnsureRunBlockedOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureBlockedOnApproval(ctx, runmodule.BlockOnApprovalCommand{
		RunID:      runID,
		ApprovalID: approvalID,
	})
}

func (p runServiceActions) EnsureRunResumed(ctx context.Context, runID common.RunID) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureResumed(ctx, runmodule.ResumeRunCommand{RunID: runID})
}

type decisionServiceActions struct {
	service decisionmodule.Service
}

func newDecisionActionPort(service decisionmodule.Service) DecisionActionRuntime {
	return decisionServiceActions{service: service}
}

func (p decisionServiceActions) EnsureTicket(
	ctx context.Context,
	ticketID common.TicketID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureTicket(ctx, decisionmodule.CreateTicketCommand{
		TicketID:    ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      reason,
	})
}

func (p decisionServiceActions) EnsureResolvedTicket(
	ctx context.Context,
	resolutionID common.ResolutionID,
	ticketID common.TicketID,
	resolutionType string,
	payload map[string]any,
) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureResolvedTicket(ctx, decisionmodule.ResolveTicketCommand{
		ResolutionID: resolutionID,
		TicketID:     ticketID,
		Type:         resolutionType,
		Payload:      payload,
	})
}

type approvalServiceActions struct {
	service approvalmodule.Service
}

func newApprovalActionPort(service approvalmodule.Service) ApprovalActionRuntime {
	return approvalServiceActions{service: service}
}

func (p approvalServiceActions) EnsureApproval(
	ctx context.Context,
	approvalID common.ApprovalID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureApproval(ctx, approvalmodule.CreateApprovalCommand{
		ApprovalID:  approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      reason,
	})
}

func (p approvalServiceActions) EnsureApprovalApproved(ctx context.Context, approvalID common.ApprovalID) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureApproved(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: approvalID})
}

func (p approvalServiceActions) EnsureApprovalRejected(ctx context.Context, approvalID common.ApprovalID) error {
	if p.service == nil {
		return common.ErrInvalidArgument
	}
	return p.service.EnsureRejected(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: approvalID})
}
