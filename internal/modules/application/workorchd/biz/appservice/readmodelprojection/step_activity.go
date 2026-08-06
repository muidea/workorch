package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageRunSteps(items []*runmodel.Step, query workorchquerycontract.RunResourcePageQuery) readmodelcore.StepPage {
	query = query.Normalized(stepPageLimit, maxStepPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectProjectedPage(items, limit, cursorAfterID,
		func(item *runmodel.Step) string { return string(item.ID) },
		func(item *runmodel.Step) workorchprojectioncontract.StepView {
			return ProjectStepView(*MapStep(item))
		},
	)
	return readmodelcore.StepPage{
		RunID: string(query.RunID),
		Steps: selected,
		CursorPage: workorchprojectioncontract.CursorPage{
			Limit:             limit,
			CursorAfterID:     cursorAfterID,
			NextCursorAfterID: nextCursor,
			HasMore:           hasMore,
		},
	}
}
