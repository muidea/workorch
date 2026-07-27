package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func PageRunSteps(items []*runmodel.Step, query readmodelcore.StepQuery) readmodelcore.StepPage {
	limit := query.Limit
	if limit <= 0 {
		limit = stepPageLimit
	}
	if limit > maxStepPageLimit {
		limit = maxStepPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, hasMore := selectRunStepPageItems(items, limit, cursorAfterID)
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return readmodelcore.StepPage{
		RunID:             strings.TrimSpace(query.RunID),
		Steps:             selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
