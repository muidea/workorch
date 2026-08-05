package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageRunResults(items []*resultmodel.Result, query workorchquerycontract.RunResourcePageQuery) readmodelcore.ResultPage {
	query = query.Normalized(resultPageLimit, maxResultPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectResultPage(items, limit, cursorAfterID)
	return readmodelcore.ResultPage{
		RunID:             string(query.RunID),
		Results:           selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
