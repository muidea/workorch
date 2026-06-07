package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func PageRunToolCalls(items []*runmodel.ToolCall, query readmodelcore.ToolCallQuery) readmodelcore.ToolCallPage {
	limit := query.Limit
	if limit <= 0 {
		limit = toolCallPageLimit
	}
	if limit > maxToolCallPageLimit {
		limit = maxToolCallPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectToolCallPage(items, limit, cursorAfterID)
	return readmodelcore.ToolCallPage{
		RunID:             strings.TrimSpace(query.RunID),
		ToolCalls:         selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
