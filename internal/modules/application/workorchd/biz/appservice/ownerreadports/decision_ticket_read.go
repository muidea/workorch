package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDecisionTicketByOwnerCommand(ctx context.Context, hub magicevent.Hub, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	if hub == nil || ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := sendMagicCommand[decisionevents.GetTicketCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandGetTicket, decisionevents.CommandDestination, "decision_ticket", string(ticketID), decisionevents.GetTicketCommand{
		WorkspaceID: workspaceID,
		TicketID:    ticketID,
	})
	if err != nil {
		return nil, err
	}
	if response.Ticket == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Ticket, nil
}
