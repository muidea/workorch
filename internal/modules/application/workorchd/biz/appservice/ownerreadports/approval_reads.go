package ownerreadports

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListApprovalsByOwnerCommand(ctx context.Context, hub events.Hub) ([]*approvalmodel.Approval, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicApprovalCommandList, "approval", "all", "", "", map[string]any{})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func ListApprovalsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*approvalmodel.Approval, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyRunID: string(runID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicApprovalCommandListByRun, "run", string(runID), workspaceID, runID, payload)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func ListApprovalsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicApprovalCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
