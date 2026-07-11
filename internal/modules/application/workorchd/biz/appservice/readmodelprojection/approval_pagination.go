package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
)

func PageApprovals(items []*approvalmodel.Approval, query appmodelcore.ApprovalQuery) ([]*approvalmodel.Approval, int, string, string, bool) {
	limit := query.Limit
	if limit <= 0 {
		limit = approvalPageLimit
	}
	if limit > maxApprovalPageLimit {
		limit = maxApprovalPageLimit
	}
	items = OrderedApprovalsForGovernancePage(items)
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectApprovalPageWindow(items, limit, cursorAfterID)
	return selected, limit, cursorAfterID, nextCursor, hasMore
}
