package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func selectPatchPage(items []*runtimeevidencemodel.Patch, limit int, cursorAfterID string) ([]projectioncontract.Patch, string, bool) {
	started := cursorAfterID == ""
	selected := make([]projectioncontract.Patch, 0, min(limit, len(items)))
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
