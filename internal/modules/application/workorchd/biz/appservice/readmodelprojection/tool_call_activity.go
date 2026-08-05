package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageRunToolCalls(items []*runtimeevidencemodel.ToolCall, query workorchquerycontract.RunResourcePageQuery) readmodelcore.ToolCallPage {
	query = query.Normalized(toolCallPageLimit, maxToolCallPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectToolCallPage(items, limit, cursorAfterID)
	return readmodelcore.ToolCallPage{
		RunID:             string(query.RunID),
		ToolCalls:         selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
