package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageRunPatches(items []*runtimeevidencemodel.Patch, query workorchquerycontract.RunResourcePageQuery) readmodelcore.PatchPage {
	query = query.Normalized(patchPageLimit, maxPatchPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectProjectedPage(items, limit, cursorAfterID,
		func(item *runtimeevidencemodel.Patch) string { return string(item.ID) },
		ProjectPatch,
	)
	return readmodelcore.PatchPage{
		RunID:   string(query.RunID),
		Patches: selected,
		CursorPage: workorchprojectioncontract.CursorPage{
			Limit:             limit,
			CursorAfterID:     cursorAfterID,
			NextCursorAfterID: nextCursor,
			HasMore:           hasMore,
		},
	}
}
