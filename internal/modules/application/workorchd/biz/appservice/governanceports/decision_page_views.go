package governanceports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/events"
)

func DecisionPage(ctx context.Context, hub events.Hub, traces governancecore.AutoGovernanceTraceReader, query appmodelcore.DecisionQuery) (appmodelcore.DecisionPage, error) {
	filtered, err := DecisionTicketsForQuery(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return appmodelcore.DecisionPage{}, err
	}
	pageTickets, limit, cursorAfterID, nextCursor, hasMore := readmodelprojection.PageDecisionTickets(filtered, query)
	resolutionMap, err := ResolutionsForTickets(ctx, hub, pageTickets)
	if err != nil {
		return appmodelcore.DecisionPage{}, err
	}
	decisions := make([]appmodelcore.Decision, 0, len(pageTickets))
	for _, item := range pageTickets {
		summary, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if err != nil {
			return appmodelcore.DecisionPage{}, err
		}
		decisions = append(decisions, readmodelprojection.ProjectDecisionView(readmodelprojection.MapDecision(item, resolutionMap[item.ID], summary)))
	}
	return appmodelcore.DecisionPage{
		WorkspaceID:       strings.TrimSpace(query.WorkspaceID),
		RunID:             strings.TrimSpace(query.RunID),
		Decisions:         decisions,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}, nil
}
