package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListApprovalsByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*approvalmodel.Approval, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandList, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListApprovalsCommand{})
	ownerevent.BindLane(event, approvalevents.TopicApprovalCommandList, "approval", "all", "", "", "")
	response, err := ownerevent.Approval(hub.Send(event), approvalevents.TopicApprovalCommandList)
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func ListApprovalsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*approvalmodel.Approval, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandListByRun, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListApprovalsByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
	ownerevent.BindLane(event, approvalevents.TopicApprovalCommandListByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Approval(hub.Send(event), approvalevents.TopicApprovalCommandListByRun)
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}

func ListApprovalsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandListByWorkspace, sourceModule, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, approvalevents.ListApprovalsByWorkspaceCommand{
		WorkspaceID: workspaceID,
	})
	ownerevent.BindLane(event, approvalevents.TopicApprovalCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Approval(hub.Send(event), approvalevents.TopicApprovalCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Approvals, nil
}
