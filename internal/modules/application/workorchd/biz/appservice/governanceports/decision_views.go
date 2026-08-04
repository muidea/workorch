package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func DecisionView(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, ticketID common.TicketID, scope common.WorkspaceID) (appmodelcore.Decision, error) {
	item, err := ReadDecisionTicket(contextWithGovernanceScope(ctx, scope), hub, ticketID)
	if err != nil {
		return appmodelcore.Decision{}, err
	}
	return DecisionViewForTicket(ctx, hub, traces, item, scope)
}

func DecisionViewForTicket(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, item *decisionmodel.Ticket, scope common.WorkspaceID) (appmodelcore.Decision, error) {
	if err := governancecore.EnsureDecisionTicketWorkspaceScope(scope, item); err != nil {
		return appmodelcore.Decision{}, err
	}
	resolution, err := ResolutionForTicket(ctx, hub, item)
	if err != nil {
		return appmodelcore.Decision{}, err
	}
	autoGovernance, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
	if err != nil {
		return appmodelcore.Decision{}, err
	}
	return readmodelprojection.MapDecision(item, resolution, autoGovernance), nil
}

func DecisionViewsForQuery(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, workspaceID string, runID string) ([]appmodelcore.Decision, error) {
	candidates, err := DecisionTicketsForQuery(ctx, hub, workspaceID, runID)
	if err != nil {
		return nil, err
	}
	ret := make([]appmodelcore.Decision, 0, len(candidates))
	for _, item := range candidates {
		view, err := DecisionViewForTicket(ctx, hub, traces, item, "")
		if err != nil {
			return nil, err
		}
		ret = append(ret, view)
	}
	return ret, nil
}
