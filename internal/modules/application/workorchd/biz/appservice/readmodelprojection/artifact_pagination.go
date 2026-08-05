package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageRunArtifacts(items []*resultmodel.Artifact, query workorchquerycontract.RunResourcePageQuery) readmodelcore.ArtifactPage {
	query = query.Normalized(artifactPageLimit, maxArtifactPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectArtifactPage(items, limit, cursorAfterID)
	return readmodelcore.ArtifactPage{
		RunID:             string(query.RunID),
		Artifacts:         selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
