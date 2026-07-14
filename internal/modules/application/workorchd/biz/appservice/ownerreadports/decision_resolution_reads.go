package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDecisionResolutionByTicketOwnerCommand(ctx context.Context, hub magicevent.Hub, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	if hub == nil || ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandGetResolution, sourceModule, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.GetResolutionCommand{
		WorkspaceID: workspaceID,
		TicketID:    ticketID,
	})
	ownerevent.BindLane(event, decisionevents.TopicDecisionCommandGetResolution, "ticket", string(ticketID), workspaceID, "", "")
	response, err := ownerevent.Decision(hub.Send(event), decisionevents.TopicDecisionCommandGetResolution)
	if err != nil {
		return nil, err
	}
	if response.Resolution == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Resolution, nil
}
