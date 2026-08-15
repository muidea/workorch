package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: artifact_pagination.go

func pageRunArtifacts(items []*resultmodel.Artifact, query workorchquerycontract.RunResourcePageQuery) readmodelcore.ArtifactPage {
	query = query.Normalized(artifactPageLimit, maxArtifactPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectProjectedPage(items, limit, cursorAfterID,
		func(item *resultmodel.Artifact) string { return string(item.ID) },
		projectArtifact,
	)
	return readmodelcore.ArtifactPage{
		RunID:     string(query.RunID),
		Artifacts: selected,
		CursorPage: workorchprojectioncontract.CursorPage{
			Limit:             limit,
			CursorAfterID:     cursorAfterID,
			NextCursorAfterID: nextCursor,
			HasMore:           hasMore,
		},
	}
}

// source: result_pagination.go

func pageRunResults(items []*resultmodel.Result, query workorchquerycontract.RunResourcePageQuery) readmodelcore.ResultPage {
	query = query.Normalized(resultPageLimit, maxResultPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectProjectedPage(items, limit, cursorAfterID,
		func(item *resultmodel.Result) string { return string(item.ID) },
		projectResult,
	)
	return readmodelcore.ResultPage{
		RunID:   string(query.RunID),
		Results: selected,
		CursorPage: workorchprojectioncontract.CursorPage{
			Limit:             limit,
			CursorAfterID:     cursorAfterID,
			NextCursorAfterID: nextCursor,
			HasMore:           hasMore,
		},
	}
}
