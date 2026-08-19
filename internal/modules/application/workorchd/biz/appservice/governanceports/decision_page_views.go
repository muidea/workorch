package governanceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func DecisionPage(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, query workorchquerycontract.RunResourcePageQuery) (appmodelcore.DecisionPage, error) {
	query = query.Normalized(decisionevents.DefaultListLimit, decisionevents.MaxListLimit)
	workspaceID := common.WorkspaceID(strings.TrimSpace(string(query.WorkspaceID)))
	runID := common.RunID(strings.TrimSpace(string(query.RunID)))
	if workspaceID != "" {
		ctx = contextWithGovernanceScope(ctx, workspaceID)
	}
	var (
		pageTickets []*decisionmodel.Ticket
		nextCursor  string
		err         error
	)
	switch {
	case runID != "" && workspaceID == "":
		return appmodelcore.DecisionPage{}, common.ErrInvalidArgument
	case runID != "":
		pageTickets, nextCursor, err = ownerreadports.ListDecisionTicketsByRunOwnerPage(ctx, hub, workspaceID, runID, query.CursorAfterID, query.Limit)
	case workspaceID != "":
		pageTickets, nextCursor, err = ownerreadports.ListDecisionTicketsByWorkspaceOwnerPage(ctx, hub, workspaceID, query.CursorAfterID, query.Limit)
	default:
		pageTickets, nextCursor, err = ownerreadports.ListDecisionTicketsOwnerPage(ctx, hub, query.CursorAfterID, query.Limit)
	}
	if err != nil {
		return appmodelcore.DecisionPage{}, err
	}
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
		decisions = append(decisions, readmodelprojection.GovernanceProjection().ProjectDecisionView(readmodelprojection.GovernanceProjection().MapDecision(item, resolutionMap[decisionResolutionMapKey(item)], summary)))
	}
	return appmodelcore.DecisionPage{
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		Decisions:   decisions,
		CursorPage: workorchprojectioncontract.CursorPage{
			Limit:             query.Limit,
			CursorAfterID:     query.CursorAfterID,
			NextCursorAfterID: nextCursor,
			HasMore:           nextCursor != "",
		},
	}, nil
}
