package runtimeports

import (
	"context"

	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
)

func (p *EventHubActionPort) EnsureTicket(
	ctx context.Context,
	ticketID common.TicketID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if p == nil || p.hub == nil || ticketID == "" || workspaceID == "" || runID == "" || stepID == "" {
		return common.ErrInvalidArgument
	}
	return p.send(ctx, decisionevents.TopicDecisionCommandEnsureTicket, decisionevents.CommandDestination, "decision_ticket", string(ticketID), workspaceID, runID, stepID, decisionevents.EnsureTicketCommand{
		TicketID:    ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      reason,
	})
}

func (p *EventHubActionPort) EnsureResolvedTicket(
	ctx context.Context,
	resolutionID common.ResolutionID,
	ticketID common.TicketID,
	resolutionType string,
	payload map[string]any,
) error {
	if p == nil || p.hub == nil || resolutionID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	return p.send(ctx, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.CommandDestination, "decision_ticket", string(ticketID), workspaceID, "", "", decisionevents.EnsureResolvedCommand{
		ResolutionID: resolutionID,
		TicketID:     ticketID,
		WorkspaceID:  workspaceID,
		Type:         resolutionType,
		Payload:      cloneActionPayload(payload),
	})
}
