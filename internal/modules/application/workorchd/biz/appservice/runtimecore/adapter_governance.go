package runtimecore

import (
	"context"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Adapter) EnsureTicket(
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

func (r *Adapter) GetTicket(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetTicket(ctx, ticketID)
}

func (r *Adapter) GetResolutionByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetResolutionByTicketID(ctx, ticketID)
}

func (r *Adapter) EnsureResolvedTicket(
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
