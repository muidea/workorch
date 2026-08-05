package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageRunSteps(items []*runmodel.Step, query workorchquerycontract.RunResourcePageQuery) readmodelcore.StepPage {
	query = query.Normalized(stepPageLimit, maxStepPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, hasMore := selectRunStepPageItems(items, limit, cursorAfterID)
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return readmodelcore.StepPage{
		RunID:             string(query.RunID),
		Steps:             selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
