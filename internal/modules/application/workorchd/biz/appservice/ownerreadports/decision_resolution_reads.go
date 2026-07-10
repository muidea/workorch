package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDecisionResolutionByTicketOwnerCommand(ctx context.Context, hub magicevent.Hub, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	if hub == nil || ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[decisionevents.GetResolutionCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandGetResolution, decisionevents.CommandDestination, "ticket", string(ticketID), decisionevents.GetResolutionCommand{
		WorkspaceID: workspaceID,
		TicketID:    ticketID,
	})
	if err != nil {
		return nil, err
	}
	if response.Resolution == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Resolution, nil
}
