package readmodelprojection

import (
	"strings"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageDecisionTickets(items []*decisionmodel.Ticket, query workorchquerycontract.RunResourcePageQuery) ([]*decisionmodel.Ticket, int, string, string, bool) {
	query = query.Normalized(decisionPageLimit, maxDecisionPageLimit)
	limit := query.Limit
	items = OrderedDecisionTicketsForGovernancePage(items)
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectDecisionPageWindow(items, limit, cursorAfterID)
	return selected, limit, cursorAfterID, nextCursor, hasMore
}
