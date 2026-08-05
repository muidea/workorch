package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageRunPatches(items []*runtimeevidencemodel.Patch, query workorchquerycontract.RunResourcePageQuery) readmodelcore.PatchPage {
	query = query.Normalized(patchPageLimit, maxPatchPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectPatchPage(items, limit, cursorAfterID)
	return readmodelcore.PatchPage{
		RunID:             string(query.RunID),
		Patches:           selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
