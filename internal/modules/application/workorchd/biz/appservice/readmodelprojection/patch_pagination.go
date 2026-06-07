package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func PageRunPatches(items []*runmodel.Patch, query readmodelcore.PatchQuery) readmodelcore.PatchPage {
	limit := query.Limit
	if limit <= 0 {
		limit = patchPageLimit
	}
	if limit > maxPatchPageLimit {
		limit = maxPatchPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectPatchPage(items, limit, cursorAfterID)
	return readmodelcore.PatchPage{
		RunID:             strings.TrimSpace(query.RunID),
		Patches:           selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
