package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func PageRunResults(items []*resultmodel.Result, query readmodelcore.ResultQuery) readmodelcore.ResultPage {
	limit := query.Limit
	if limit <= 0 {
		limit = resultPageLimit
	}
	if limit > maxResultPageLimit {
		limit = maxResultPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectResultPage(items, limit, cursorAfterID)
	return readmodelcore.ResultPage{
		RunID:             strings.TrimSpace(query.RunID),
		Results:           selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
