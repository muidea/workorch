package governanceports

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadDecisionResolutionByTicket(ctx context.Context, hub events.Hub, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	return ownerreadports.ReadDecisionResolutionByTicketOwnerCommand(ctx, hub, ticketID)
}

func ResolutionForTicket(ctx context.Context, hub events.Hub, item *decisionmodel.Ticket) (*decisionmodel.Resolution, error) {
	if item == nil || hub == nil || item.State != common.DecisionTicketStateResolved {
		return nil, nil
	}
	resolution, err := ReadDecisionResolutionByTicket(ctx, hub, item.ID)
	if errors.Is(err, common.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resolution, nil
}

func ResolutionsForTickets(ctx context.Context, hub events.Hub, items []*decisionmodel.Ticket) (map[common.TicketID]*decisionmodel.Resolution, error) {
	ret := make(map[common.TicketID]*decisionmodel.Resolution, len(items))
	for _, item := range items {
		if item == nil || item.State != common.DecisionTicketStateResolved {
			continue
		}
		resolution, err := ResolutionForTicket(ctx, hub, item)
		if err != nil {
			return nil, err
		}
		if resolution != nil {
			ret[item.ID] = resolution
		}
	}
	return ret, nil
}
