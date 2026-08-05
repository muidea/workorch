package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageRunRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint, query workorchquerycontract.RunResourcePageQuery) readmodelcore.RuntimeCheckpointPage {
	query = query.Normalized(checkpointPageLimit, maxCheckpointPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectRuntimeCheckpointPage(items, limit, cursorAfterID)
	return readmodelcore.RuntimeCheckpointPage{
		RunID:              string(query.RunID),
		RuntimeCheckpoints: selected,
		Limit:              limit,
		CursorAfterID:      cursorAfterID,
		NextCursorAfterID:  nextCursor,
		HasMore:            hasMore,
	}
}
