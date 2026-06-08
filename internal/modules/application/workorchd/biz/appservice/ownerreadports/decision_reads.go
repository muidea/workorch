package ownerreadports

import (
	"context"

	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListDecisionTicketsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, decisionevents.TopicDecisionCommandListTicketsByWS, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func ListDecisionTicketsByOwnerCommand(ctx context.Context, hub events.Hub) ([]*decisionmodel.Ticket, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, decisionevents.TopicDecisionCommandListTickets, "decision_ticket", "all", "", "", map[string]any{})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func ListDecisionTicketsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyRunID: string(runID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := PublishCommandWithResponse(ctx, hub, decisionevents.TopicDecisionCommandListTicketsByRun, "run", string(runID), workspaceID, runID, payload)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
