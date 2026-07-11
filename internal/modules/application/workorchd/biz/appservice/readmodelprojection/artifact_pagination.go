package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func PageRunArtifacts(items []*resultmodel.Artifact, query readmodelcore.ArtifactQuery) readmodelcore.ArtifactPage {
	limit := query.Limit
	if limit <= 0 {
		limit = artifactPageLimit
	}
	if limit > maxArtifactPageLimit {
		limit = maxArtifactPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectArtifactPage(items, limit, cursorAfterID)
	return readmodelcore.ArtifactPage{
		RunID:             strings.TrimSpace(query.RunID),
		Artifacts:         selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
