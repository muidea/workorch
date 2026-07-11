package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

func PageDecisionTickets(items []*decisionmodel.Ticket, query appmodelcore.DecisionQuery) ([]*decisionmodel.Ticket, int, string, string, bool) {
	limit := query.Limit
	if limit <= 0 {
		limit = decisionPageLimit
	}
	if limit > maxDecisionPageLimit {
		limit = maxDecisionPageLimit
	}
	items = OrderedDecisionTicketsForGovernancePage(items)
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectDecisionPageWindow(items, limit, cursorAfterID)
	return selected, limit, cursorAfterID, nextCursor, hasMore
}
