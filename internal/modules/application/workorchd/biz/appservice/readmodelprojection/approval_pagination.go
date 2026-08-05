package readmodelprojection

import (
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageApprovals(items []*approvalmodel.Approval, query workorchquerycontract.RunResourcePageQuery) ([]*approvalmodel.Approval, int, string, string, bool) {
	query = query.Normalized(approvalPageLimit, maxApprovalPageLimit)
	limit := query.Limit
	items = OrderedApprovalsForGovernancePage(items)
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectApprovalPageWindow(items, limit, cursorAfterID)
	return selected, limit, cursorAfterID, nextCursor, hasMore
}
