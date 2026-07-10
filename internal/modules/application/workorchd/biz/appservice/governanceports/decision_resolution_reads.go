package governanceports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDecisionResolutionByTicket(ctx context.Context, hub magicevent.Hub, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	return ownerreadports.ReadDecisionResolutionByTicketOwnerCommand(ctx, hub, ticketID)
}

func ResolutionForTicket(ctx context.Context, hub magicevent.Hub, item *decisionmodel.Ticket) (*decisionmodel.Resolution, error) {
	if item == nil || hub == nil || item.State != common.DecisionTicketStateResolved {
		return nil, nil
	}
	resolutionCtx := contextWithGovernanceScope(ctx, item.WorkspaceID)
	resolution, err := ReadDecisionResolutionByTicket(resolutionCtx, hub, item.ID)
	if errors.Is(err, common.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resolution, nil
}

func ResolutionsForTickets(ctx context.Context, hub magicevent.Hub, items []*decisionmodel.Ticket) (map[string]*decisionmodel.Resolution, error) {
	ret := make(map[string]*decisionmodel.Resolution, len(items))
	for _, item := range items {
		if item == nil || item.State != common.DecisionTicketStateResolved {
			continue
		}
		resolution, err := ResolutionForTicket(ctx, hub, item)
		if err != nil {
			return nil, err
		}
		if resolution != nil {
			ret[decisionResolutionMapKey(item)] = resolution
		}
	}
	return ret, nil
}

func decisionResolutionMapKey(item *decisionmodel.Ticket) string {
	if item == nil {
		return ""
	}
	return string(item.WorkspaceID) + "\x00" + string(item.ID)
}
