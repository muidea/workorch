package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDecisionTicketByOwnerCommand(ctx context.Context, hub magicevent.Hub, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	if hub == nil || ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandGetTicket, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.GetTicketCommand{
		WorkspaceID: workspaceID,
		TicketID:    ticketID,
	})
	ownerevent.BindLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandGetTicket, "decision_ticket", string(ticketID), workspaceID, "", "")
	response, err := ownerevent.Decision(hub.Send(event), decisionevents.TopicDecisionCommandGetTicket)
	if err != nil {
		return nil, err
	}
	if response.Ticket == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Ticket, nil
}
