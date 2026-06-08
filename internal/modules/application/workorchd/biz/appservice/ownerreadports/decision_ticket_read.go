package ownerreadports

import (
	"context"

	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadDecisionTicketByOwnerCommand(ctx context.Context, hub events.Hub, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	if hub == nil || ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyBlockingTicketID: string(ticketID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := PublishCommandWithResponse(ctx, hub, decisionevents.TopicDecisionCommandGetTicket, "decision_ticket", string(ticketID), workspaceID, "", payload)
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyTicket].(*decisionmodel.Ticket)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}
