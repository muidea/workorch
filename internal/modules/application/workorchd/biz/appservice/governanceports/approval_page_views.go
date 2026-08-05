package governanceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func ApprovalPage(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, query workorchquerycontract.RunResourcePageQuery) (appmodelcore.ApprovalPage, error) {
	filtered, err := ApprovalsForQuery(ctx, hub, string(query.WorkspaceID), string(query.RunID))
	if err != nil {
		return appmodelcore.ApprovalPage{}, err
	}
	pageApprovals, limit, cursorAfterID, nextCursor, hasMore := readmodelprojection.PageApprovals(filtered, query)
	approvals := make([]workorchprojectioncontract.Approval, 0, len(pageApprovals))
	for _, item := range pageApprovals {
		summary, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
		if err != nil {
			return appmodelcore.ApprovalPage{}, err
		}
		approvals = append(approvals, readmodelprojection.ProjectApprovalView(readmodelprojection.MapApproval(item, summary)))
	}
	return appmodelcore.ApprovalPage{
		WorkspaceID:       strings.TrimSpace(string(query.WorkspaceID)),
		RunID:             strings.TrimSpace(string(query.RunID)),
		Approvals:         approvals,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}, nil
}
