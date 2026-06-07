package readmodelprojection

import (
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
)

func selectDecisionPageWindow(items []*decisionmodel.Ticket, limit int, cursorAfterID string) ([]*decisionmodel.Ticket, string, bool) {
	started := cursorAfterID == ""
	selected := make([]*decisionmodel.Ticket, 0, min(limit, len(items)))
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
