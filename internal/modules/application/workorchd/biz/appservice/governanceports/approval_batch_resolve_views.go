package governanceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func BatchResolveApprovalViews(ctx context.Context, hub events.Hub, traces governancecore.AutoGovernanceTraceReader, cmd appmodelcore.BatchResolveApprovalCommand, scope common.WorkspaceID) ([]readmodelcore.Approval, error) {
	action, err := governancecore.NormalizeApprovalBatchAction(cmd.Action)
	if err != nil {
		return nil, err
	}
	if err := governancecore.ValidateApprovalIDs(cmd.ApprovalIDs); err != nil {
		return nil, err
	}
	type approvalRead struct {
		item *approvalmodel.Approval
		err  error
	}
	readCtx := contextWithGovernanceScope(ctx, scope)
	reads := make([]approvalRead, len(cmd.ApprovalIDs))
	for i, approvalID := range cmd.ApprovalIDs {
		reads[i].item, reads[i].err = ReadApproval(readCtx, hub, common.ApprovalID(approvalID))
	}
	for _, r := range reads {
		if r.err != nil {
			return nil, r.err
		}
		if err := governancecore.EnsureApprovalWorkspaceScope(scope, r.item); err != nil {
			return nil, err
		}
		if r.item.State != common.ApprovalStatePending {
			return nil, common.ErrConflict
		}
	}

	ret := make([]readmodelcore.Approval, 0, len(cmd.ApprovalIDs))
	for _, r := range reads {
		item, err := resolveApprovalItemByAction(ctx, hub, r.item, action)
		if err != nil {
			return nil, err
		}
		view, err := ApprovalViewForItem(ctx, traces, item, scope)
		if err != nil {
			return nil, err
		}
		ret = append(ret, view)
	}
	return ret, nil
}
