package ownerreadports

import (
	"context"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadApprovalByOwnerCommand(ctx context.Context, hub events.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	if hub == nil || approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	response, err := sendMagicCommand[approvalevents.GetApprovalCommand, approvalevents.ApprovalCommandResult](ctx, hub, approvalevents.TopicApprovalCommandGet, approvalevents.CommandDestination, "approval", string(approvalID), approvalevents.GetApprovalCommand{
		WorkspaceID: workspaceID,
		ApprovalID:  approvalID,
	})
	if err != nil {
		return nil, err
	}
	if response.Approval == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Approval, nil
}
