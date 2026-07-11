package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ApproveApproval(ctx context.Context, hub magicevent.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return resolveApproval(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, approvalevents.EnsureApprovedCommand{ApprovalID: approvalID})
}

func RejectApproval(ctx context.Context, hub magicevent.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return resolveApproval(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, approvalevents.EnsureRejectedCommand{ApprovalID: approvalID})
}

func ApproveApprovalItem(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, item, nil)
}

func RejectApprovalItem(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, item, nil)
}

func ResolveApprovalItemWithContext(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval, action string, resolutionContext map[string]any) (*approvalmodel.Approval, error) {
	if action == "approve" {
		return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, item, resolutionContext)
	}
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, item, resolutionContext)
}

func resolveApprovalItem(ctx context.Context, hub magicevent.Hub, topic string, item *approvalmodel.Approval, resolutionContext map[string]any) (*approvalmodel.Approval, error) {
	if item == nil || item.ID == "" {
		return nil, common.ErrInvalidArgument
	}
	return resolveApproval(ctx, hub, topic, approvalevents.EnsureApprovedCommand{
		ApprovalID:  item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		Context:     governancecore.CloneApprovalMap(resolutionContext),
	})
}

func resolveApproval(ctx context.Context, hub magicevent.Hub, topic string, cmd approvalevents.EnsureApprovedCommand) (*approvalmodel.Approval, error) {
	response, err := sendGovernanceMagicCommand[approvalevents.EnsureApprovedCommand, approvalevents.ApprovalCommandResult](ctx, hub, topic, approvalevents.CommandDestination, "approval", string(cmd.ApprovalID), cmd.WorkspaceID, cmd.RunID, cmd)
	if err != nil {
		return nil, err
	}
	if response.Approval == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Approval, nil
}
