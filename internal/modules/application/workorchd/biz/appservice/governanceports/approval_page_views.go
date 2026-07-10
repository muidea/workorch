package governanceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
)

func ApprovalPage(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, query appmodelcore.ApprovalQuery) (appmodelcore.ApprovalPage, error) {
	filtered, err := ApprovalsForQuery(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return appmodelcore.ApprovalPage{}, err
	}
	pageApprovals, limit, cursorAfterID, nextCursor, hasMore := readmodelprojection.PageApprovals(filtered, query)
	approvals := make([]appmodelcore.Approval, 0, len(pageApprovals))
	for _, item := range pageApprovals {
		summary, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
		if err != nil {
			return appmodelcore.ApprovalPage{}, err
		}
		approvals = append(approvals, readmodelprojection.ProjectApprovalView(readmodelprojection.MapApproval(item, summary)))
	}
	return appmodelcore.ApprovalPage{
		WorkspaceID:       strings.TrimSpace(query.WorkspaceID),
		RunID:             strings.TrimSpace(query.RunID),
		Approvals:         approvals,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}, nil
}
