package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func selectArtifactPage(items []*resultmodel.Artifact, limit int, cursorAfterID string) ([]projectioncontract.Artifact, string, bool) {
	started := cursorAfterID == ""
	selected := make([]projectioncontract.Artifact, 0, min(limit, len(items)))
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
		selected = append(selected, ProjectArtifact(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return selected, nextCursor, hasMore
}
