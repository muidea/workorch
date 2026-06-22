package governanceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ApproveApproval(ctx context.Context, hub events.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return resolveApproval(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, approvalcontracts.ResolveApprovalCommand{ApprovalID: approvalID})
}

func RejectApproval(ctx context.Context, hub events.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return resolveApproval(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, approvalcontracts.ResolveApprovalCommand{ApprovalID: approvalID})
}

func ApproveApprovalItem(ctx context.Context, hub events.Hub, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, item, nil)
}

func RejectApprovalItem(ctx context.Context, hub events.Hub, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, item, nil)
}

func ResolveApprovalItemWithContext(ctx context.Context, hub events.Hub, item *approvalmodel.Approval, action string, resolutionContext map[string]any) (*approvalmodel.Approval, error) {
	if action == "approve" {
		return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, item, resolutionContext)
	}
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, item, resolutionContext)
}

func resolveApprovalItem(ctx context.Context, hub events.Hub, topic string, item *approvalmodel.Approval, resolutionContext map[string]any) (*approvalmodel.Approval, error) {
	if item == nil || item.ID == "" {
		return nil, common.ErrInvalidArgument
	}
	return resolveApproval(ctx, hub, topic, approvalcontracts.ResolveApprovalCommand{
		ApprovalID:  item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		Context:     governancecore.CloneApprovalMap(resolutionContext),
	})
}

func resolveApproval(ctx context.Context, hub events.Hub, topic string, cmd approvalcontracts.ResolveApprovalCommand) (*approvalmodel.Approval, error) {
	payload := map[string]any{
		events.PayloadKeyCommand: cmd,
	}
	if cmd.WorkspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(cmd.WorkspaceID)
	}
	if cmd.RunID != "" {
		payload[events.PayloadKeyRunID] = string(cmd.RunID)
	}
	response, err := ownercommandports.PublishWithResponse(ctx, hub, topic, "approval", string(cmd.ApprovalID), cmd.WorkspaceID, cmd.RunID, payload)
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyApproval].(*approvalmodel.Approval)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}
