package governanceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ResolveApprovalView(ctx context.Context, hub events.Hub, traces governancecore.AutoGovernanceTraceReader, approvalID common.ApprovalID, action string, scope common.WorkspaceID) (readmodelcore.Approval, error) {
	action, err := governancecore.NormalizeApprovalBatchAction(action)
	if err != nil {
		return readmodelcore.Approval{}, err
	}
	if scope != "" {
		existing, err := ReadApproval(ctx, hub, approvalID)
		if err != nil {
			return readmodelcore.Approval{}, err
		}
		if err := governancecore.EnsureApprovalWorkspaceScope(scope, existing); err != nil {
			return readmodelcore.Approval{}, err
		}
	}
	var item *approvalmodel.Approval
	if action == "approve" {
		item, err = ApproveApproval(ctx, hub, approvalID)
	} else {
		item, err = RejectApproval(ctx, hub, approvalID)
	}
	if err != nil {
		return readmodelcore.Approval{}, err
	}
	return ApprovalViewForItem(ctx, traces, item, scope)
}

func resolveApprovalByAction(ctx context.Context, hub events.Hub, approvalID common.ApprovalID, action string) (*approvalmodel.Approval, error) {
	if action == "approve" {
		return ApproveApproval(ctx, hub, approvalID)
	}
	return RejectApproval(ctx, hub, approvalID)
}
