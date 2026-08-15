package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: patch_pagination.go

func pageRunPatches(items []*runtimeevidencemodel.Patch, query workorchquerycontract.RunResourcePageQuery) readmodelcore.PatchPage {
	query = query.Normalized(patchPageLimit, maxPatchPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectProjectedPage(items, limit, cursorAfterID,
		func(item *runtimeevidencemodel.Patch) string { return string(item.ID) },
		projectPatch,
	)
	return readmodelcore.PatchPage{
		RunID:   string(query.RunID),
		Patches: selected,
		CursorPage: workorchprojectioncontract.CursorPage{
			Limit:             limit,
			CursorAfterID:     cursorAfterID,
			NextCursorAfterID: nextCursor,
			HasMore:           hasMore,
		},
	}
}

// source: tool_call_activity.go

func pageRunToolCalls(items []*runtimeevidencemodel.ToolCall, query workorchquerycontract.RunResourcePageQuery) readmodelcore.ToolCallPage {
	query = query.Normalized(toolCallPageLimit, maxToolCallPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectProjectedPage(items, limit, cursorAfterID,
		func(item *runtimeevidencemodel.ToolCall) string { return string(item.ID) },
		projectToolCall,
	)
	return readmodelcore.ToolCallPage{
		RunID:     string(query.RunID),
		ToolCalls: selected,
		CursorPage: workorchprojectioncontract.CursorPage{
			Limit:             limit,
			CursorAfterID:     cursorAfterID,
			NextCursorAfterID: nextCursor,
			HasMore:           hasMore,
		},
	}
}

// source: validation_pagination.go

func pageRunValidationResults(items []*runtimeevidencemodel.ValidationResult, query workorchquerycontract.RunResourcePageQuery) readmodelcore.ValidationResultPage {
	query = query.Normalized(validationPageLimit, maxValidationPageLimit)
	limit := query.Limit
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	selected, nextCursor, hasMore := selectProjectedPage(items, limit, cursorAfterID,
		func(item *runtimeevidencemodel.ValidationResult) string { return string(item.ID) },
		projectValidationResult,
	)
	return readmodelcore.ValidationResultPage{
		RunID:             string(query.RunID),
		ValidationResults: selected,
		CursorPage: workorchprojectioncontract.CursorPage{
			Limit:             limit,
			CursorAfterID:     cursorAfterID,
			NextCursorAfterID: nextCursor,
			HasMore:           hasMore,
		},
	}
}
