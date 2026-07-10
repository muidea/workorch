package governanceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDecisionTicket(ctx context.Context, hub magicevent.Hub, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	return ownerreadports.ReadDecisionTicketByOwnerCommand(ctx, hub, ticketID)
}

func ListDecisionTickets(ctx context.Context, hub magicevent.Hub) ([]*decisionmodel.Ticket, error) {
	return ownerreadports.ListDecisionTicketsByOwnerCommand(ctx, hub)
}

func ListDecisionTicketsByWorkspace(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	return ownerreadports.ListDecisionTicketsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
}

func ListDecisionTicketsByRun(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	return ownerreadports.ListDecisionTicketsByRunOwnerCommand(ctx, hub, runID)
}

func DecisionTicketsForQuery(ctx context.Context, hub magicevent.Hub, workspaceID string, runID string) ([]*decisionmodel.Ticket, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	if workspaceID != "" {
		ctx = contextWithGovernanceScope(ctx, common.WorkspaceID(workspaceID))
	}
	var (
		items []*decisionmodel.Ticket
		err   error
	)
	switch {
	case runID != "":
		items, err = ListDecisionTicketsByRun(ctx, hub, common.RunID(runID))
	case workspaceID != "":
		items, err = ListDecisionTicketsByWorkspace(ctx, hub, common.WorkspaceID(workspaceID))
	default:
		items, err = ListDecisionTickets(ctx, hub)
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]*decisionmodel.Ticket, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if workspaceID != "" && string(item.WorkspaceID) != workspaceID {
			continue
		}
		if runID != "" && string(item.RunID) != runID {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}
