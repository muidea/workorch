package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func PageRunRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint, query readmodelcore.RuntimeCheckpointQuery) readmodelcore.RuntimeCheckpointPage {
	limit := query.Limit
	if limit <= 0 {
		limit = checkpointPageLimit
	}
	if limit > maxCheckpointPageLimit {
		limit = maxCheckpointPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectRuntimeCheckpointPage(items, limit, cursorAfterID)
	return readmodelcore.RuntimeCheckpointPage{
		RunID:              strings.TrimSpace(query.RunID),
		RuntimeCheckpoints: selected,
		Limit:              limit,
		CursorAfterID:      cursorAfterID,
		NextCursorAfterID:  nextCursor,
		HasMore:            hasMore,
	}
}
