package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func PageRunValidationResults(items []*runtimeevidencemodel.ValidationResult, query workorchquerycontract.RunResourcePageQuery) readmodelcore.ValidationResultPage {
	query = query.Normalized(validationPageLimit, maxValidationPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectValidationResultPage(items, limit, cursorAfterID)
	return readmodelcore.ValidationResultPage{
		RunID:             string(query.RunID),
		ValidationResults: selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}
