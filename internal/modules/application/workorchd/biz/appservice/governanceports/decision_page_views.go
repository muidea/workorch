package governanceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func DecisionPage(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, query workorchquerycontract.RunResourcePageQuery) (appmodelcore.DecisionPage, error) {
	filtered, err := DecisionTicketsForQuery(ctx, hub, string(query.WorkspaceID), string(query.RunID))
	if err != nil {
		return appmodelcore.DecisionPage{}, err
	}
	pageTickets, limit, cursorAfterID, nextCursor, hasMore := readmodelprojection.PageDecisionTickets(filtered, query)
	resolutionMap, err := ResolutionsForTickets(ctx, hub, pageTickets)
	if err != nil {
		return appmodelcore.DecisionPage{}, err
	}
	decisions := make([]workorchprojectioncontract.Decision, 0, len(pageTickets))
	for _, item := range pageTickets {
		summary, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if err != nil {
			return appmodelcore.DecisionPage{}, err
		}
		decisions = append(decisions, readmodelprojection.ProjectDecisionView(readmodelprojection.MapDecision(item, resolutionMap[decisionResolutionMapKey(item)], summary)))
	}
	return appmodelcore.DecisionPage{
		WorkspaceID:       strings.TrimSpace(string(query.WorkspaceID)),
		RunID:             strings.TrimSpace(string(query.RunID)),
		Decisions:         decisions,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}, nil
}
