package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListDecisionTicketsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTicketsByWS, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsByWorkspaceCommand{
		WorkspaceID: workspaceID,
	})
	ownerevent.BindLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandListTicketsByWS, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Decision(hub.Send(event), decisionevents.TopicDecisionCommandListTicketsByWS)
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}

func ListDecisionTicketsByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*decisionmodel.Ticket, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTickets, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsCommand{})
	ownerevent.BindLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandListTickets, "decision_ticket", "all", "", "", "")
	response, err := ownerevent.Decision(hub.Send(event), decisionevents.TopicDecisionCommandListTickets)
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}

func ListDecisionTicketsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandListTicketsByRun, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.ListTicketsByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
	ownerevent.BindLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandListTicketsByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Decision(hub.Send(event), decisionevents.TopicDecisionCommandListTicketsByRun)
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}
