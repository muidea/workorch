package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func selectRuntimeCheckpointPage(items []*runmodel.RuntimeCheckpoint, limit int, cursorAfterID string) ([]projectioncontract.RuntimeCheckpoint, string, bool) {
	started := cursorAfterID == ""
	selected := make([]projectioncontract.RuntimeCheckpoint, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, ProjectRuntimeCheckpoint(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return selected, nextCursor, hasMore
}
