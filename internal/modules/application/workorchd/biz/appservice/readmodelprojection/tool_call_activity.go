package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func PageRunToolCalls(items []*runtimeevidencemodel.ToolCall, query readmodelcore.ToolCallQuery) readmodelcore.ToolCallPage {
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
