package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadApprovalByOwnerCommand(ctx context.Context, hub magicevent.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	if hub == nil || approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandGet, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.GetApprovalCommand{
		WorkspaceID: workspaceID,
		ApprovalID:  approvalID,
	})
	ownerevent.BindLane(event, approvalevents.TopicApprovalCommandGet, "approval", string(approvalID), workspaceID, "", "")
	response, err := ownerevent.Approval(hub.Send(event), approvalevents.TopicApprovalCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Approval == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Approval, nil
}
