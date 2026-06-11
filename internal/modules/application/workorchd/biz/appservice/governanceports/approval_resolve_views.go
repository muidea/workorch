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
	var existing *approvalmodel.Approval
	if scope != "" {
		existing, err = ReadApproval(contextWithGovernanceScope(ctx, scope), hub, approvalID)
		if err != nil {
			return readmodelcore.Approval{}, err
		}
		if err := governancecore.EnsureApprovalWorkspaceScope(scope, existing); err != nil {
			return readmodelcore.Approval{}, err
		}
	}
	var item *approvalmodel.Approval
	if action == "approve" {
		if existing != nil {
			item, err = ApproveApprovalItem(ctx, hub, existing)
		} else {
			item, err = ApproveApproval(ctx, hub, approvalID)
		}
	} else {
		if existing != nil {
			item, err = RejectApprovalItem(ctx, hub, existing)
		} else {
			item, err = RejectApproval(ctx, hub, approvalID)
		}
	}
	if err != nil {
		return readmodelcore.Approval{}, err
	}
	return ApprovalViewForItem(ctx, traces, item, scope)
}

func resolveApprovalItemByAction(ctx context.Context, hub events.Hub, item *approvalmodel.Approval, action string) (*approvalmodel.Approval, error) {
	if action == "approve" {
		return ApproveApprovalItem(ctx, hub, item)
	}
	return RejectApprovalItem(ctx, hub, item)
}
