package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListDecisionTicketsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[decisionevents.ListTicketsByWorkspaceCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandListTicketsByWS, decisionevents.CommandDestination, "workspace", string(workspaceID), decisionevents.ListTicketsByWorkspaceCommand{
		WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}

func ListDecisionTicketsByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*decisionmodel.Ticket, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[decisionevents.ListTicketsCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandListTickets, decisionevents.CommandDestination, "decision_ticket", "all", decisionevents.ListTicketsCommand{})
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
	response, err := sendMagicCommand[decisionevents.ListTicketsByRunCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandListTicketsByRun, decisionevents.CommandDestination, "run", string(runID), decisionevents.ListTicketsByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
	if err != nil {
		return nil, err
	}
	return response.Tickets, nil
}
