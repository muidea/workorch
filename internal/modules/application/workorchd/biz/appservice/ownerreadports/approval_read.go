package ownerreadports

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadApprovalByOwnerCommand(ctx context.Context, hub events.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	if hub == nil || approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyApprovalID: string(approvalID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicApprovalCommandGet, "approval", string(approvalID), workspaceID, "", payload)
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyApproval].(*approvalmodel.Approval)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}
