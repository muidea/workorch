package readmodelprojection

import approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"

func selectApprovalPageWindow(items []*approvalmodel.Approval, limit int, cursorAfterID string) ([]*approvalmodel.Approval, string, bool) {
	started := cursorAfterID == ""
	selected := make([]*approvalmodel.Approval, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, item)
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = string(selected[len(selected)-1].ID)
	}
	return selected, nextCursor, hasMore
}
