package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ApprovalView(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, approvalID common.ApprovalID, scope common.WorkspaceID) (readmodelcore.Approval, error) {
	item, err := ReadApproval(contextWithGovernanceScope(ctx, scope), hub, approvalID)
	if err != nil {
		return readmodelcore.Approval{}, err
	}
	return ApprovalViewForItem(ctx, traces, item, scope)
}

func ApprovalViewForItem(ctx context.Context, traces governancecore.AutoGovernanceTraceReader, item *approvalmodel.Approval, scope common.WorkspaceID) (readmodelcore.Approval, error) {
	if err := governancecore.EnsureApprovalWorkspaceScope(scope, item); err != nil {
		return readmodelcore.Approval{}, err
	}
	autoGovernance, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
	if err != nil {
		return readmodelcore.Approval{}, err
	}
	return readmodelprojection.MapApproval(item, autoGovernance), nil
}

func ApprovalViewsForQuery(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, workspaceID string, runID string) ([]readmodelcore.Approval, error) {
	items, err := ApprovalsForQuery(ctx, hub, workspaceID, runID)
	if err != nil {
		return nil, err
	}
	ret := make([]readmodelcore.Approval, 0, len(items))
	for _, item := range items {
		view, err := ApprovalViewForItem(ctx, traces, item, "")
		if err != nil {
			return nil, err
		}
		ret = append(ret, view)
	}
	return ret, nil
}
