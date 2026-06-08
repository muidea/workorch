package ownerreadports

import (
	"context"

	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadDecisionResolutionByTicketOwnerCommand(ctx context.Context, hub events.Hub, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	if hub == nil || ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, decisionevents.TopicDecisionCommandGetResolution, "ticket", string(ticketID), "", "", map[string]any{
		events.PayloadKeyBlockingTicketID: string(ticketID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyResolution].(*decisionmodel.Resolution)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}
