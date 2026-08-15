package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: checkpoint_pagination.go

func pageRunRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint, query workorchquerycontract.RunResourcePageQuery) readmodelcore.RuntimeCheckpointPage {
	query = query.Normalized(checkpointPageLimit, maxCheckpointPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectProjectedPage(items, limit, cursorAfterID,
		func(item *runmodel.RuntimeCheckpoint) string { return string(item.ID) },
		projectRuntimeCheckpoint,
	)
	return readmodelcore.RuntimeCheckpointPage{
		RunID:              string(query.RunID),
		RuntimeCheckpoints: selected,
		CursorPage: workorchprojectioncontract.CursorPage{
			Limit:             limit,
			CursorAfterID:     cursorAfterID,
			NextCursorAfterID: nextCursor,
			HasMore:           hasMore,
		},
	}
}

// source: step_activity.go

func pageRunSteps(items []*runmodel.Step, query workorchquerycontract.RunResourcePageQuery) readmodelcore.StepPage {
	query = query.Normalized(stepPageLimit, maxStepPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectProjectedPage(items, limit, cursorAfterID,
		func(item *runmodel.Step) string { return string(item.ID) },
		func(item *runmodel.Step) workorchprojectioncontract.StepView {
			return projectStepView(*mapStep(item))
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
