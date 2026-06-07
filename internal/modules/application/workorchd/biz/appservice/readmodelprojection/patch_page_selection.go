package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func selectPatchPage(items []*runmodel.Patch, limit int, cursorAfterID string) ([]readmodelcore.Patch, string, bool) {
	started := cursorAfterID == ""
	selected := make([]readmodelcore.Patch, 0, min(limit, len(items)))
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
		selected = append(selected, ProjectPatch(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return selected, nextCursor, hasMore
}
