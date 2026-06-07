package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func PageRunValidationResults(items []*runmodel.ValidationResult, query readmodelcore.ValidationResultQuery) readmodelcore.ValidationResultPage {
	limit := query.Limit
	if limit <= 0 {
		limit = validationPageLimit
	}
	if limit > maxValidationPageLimit {
		limit = maxValidationPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectValidationResultPage(items, limit, cursorAfterID)
	return readmodelcore.ValidationResultPage{
		RunID:             strings.TrimSpace(query.RunID),
		ValidationResults: selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
