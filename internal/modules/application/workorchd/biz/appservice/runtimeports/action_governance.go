package runtimeports

import (
	"context"

	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
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
	return p.publish(ctx, events.TopicDecisionCommandEnsureTicket, "decision_ticket", string(ticketID), workspaceID, runID, stepID, map[string]any{
		events.PayloadKeyCommand: decisioncontracts.CreateTicketCommand{
			TicketID:    ticketID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      stepID,
			Reason:      reason,
		},
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
	return p.publish(ctx, events.TopicDecisionCommandEnsureResolved, "decision_ticket", string(ticketID), "", "", "", map[string]any{
		events.PayloadKeyCommand: decisioncontracts.ResolveTicketCommand{
			ResolutionID: resolutionID,
			TicketID:     ticketID,
			Type:         resolutionType,
			Payload:      cloneActionPayload(payload),
		},
	})
}
