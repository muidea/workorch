package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func ApproveApproval(ctx context.Context, hub magicevent.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return resolveApproval(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, approvalevents.EnsureApprovedCommand{ApprovalID: approvalID})
}

func RejectApproval(ctx context.Context, hub magicevent.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return resolveApproval(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, approvalevents.EnsureRejectedCommand{ApprovalID: approvalID})
}

func ApproveApprovalItem(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, item, workorcheventpayload.Payload{})
}

func RejectApprovalItem(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, item, workorcheventpayload.Payload{})
}

func ResolveApprovalItemWithContext(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval, action string, resolutionContext workorcheventpayload.Payload) (*approvalmodel.Approval, error) {
	if action == "approve" {
		return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, item, resolutionContext)
	}
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, item, resolutionContext)
}

func resolveApprovalItem(ctx context.Context, hub magicevent.Hub, topic string, item *approvalmodel.Approval, resolutionContext workorcheventpayload.Payload) (*approvalmodel.Approval, error) {
	if item == nil || item.ID == "" {
		return nil, common.ErrInvalidArgument
	}
	return resolveApproval(ctx, hub, topic, approvalevents.EnsureApprovedCommand{
		ApprovalID:  item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		Context:     resolutionContext.Clone().Fields,
	})
}

func resolveApproval(ctx context.Context, hub magicevent.Hub, topic string, cmd approvalevents.EnsureApprovedCommand) (*approvalmodel.Approval, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(topic, "application/workorchd/appservice", approvalevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
	ownerevent.BindLane(event, approvalevents.Contracts(), topic, "approval", string(cmd.ApprovalID), cmd.WorkspaceID, cmd.RunID, "")
	response, err := ownerevent.Approval(hub.Send(event), topic)
	if err != nil {
		return nil, err
	}
	if response.Approval == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Approval, nil
}
