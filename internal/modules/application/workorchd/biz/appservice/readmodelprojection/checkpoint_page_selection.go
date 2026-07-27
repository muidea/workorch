package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func selectRuntimeCheckpointPage(items []*runmodel.RuntimeCheckpoint, limit int, cursorAfterID string) ([]readmodelcore.RuntimeCheckpoint, string, bool) {
	started := cursorAfterID == ""
	selected := make([]readmodelcore.RuntimeCheckpoint, 0, min(limit, len(items)))
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
