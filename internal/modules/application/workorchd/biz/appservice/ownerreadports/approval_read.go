package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadApprovalByOwnerCommand(ctx context.Context, hub magicevent.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	if hub == nil || approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	var (
		items []*approvalmodel.Approval
		err   error
	)
	if workspaceID == "" {
		// ID-only application routes may not carry a workspace scope. The owner
		// exposes this explicit, bounded global query solely for discovery; any
		// subsequent mutation is sent with the discovered workspace/run scope.
		items, err = ListApprovalsByOwnerCommand(ctx, hub)
	} else {
		items, err = ListApprovalsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
	}
	if err != nil {
		return nil, err
	}
	var match *approvalmodel.Approval
	for _, item := range items {
		if item == nil || item.ID != approvalID {
			continue
		}
		if match != nil {
			return nil, common.ErrConflict
		}
		match = item
	}
	if match == nil {
		return nil, common.ErrNotFound
	}
	return match, nil
}
