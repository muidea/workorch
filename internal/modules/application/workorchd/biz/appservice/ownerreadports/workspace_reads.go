package ownerreadports

import (
	"context"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadWorkspaceByOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandGet, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyWorkspace].(*workspacemodel.Workspace)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ListWorkspacesByOwnerCommand(ctx context.Context, hub events.Hub) ([]*workspacemodel.Workspace, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandList, "workspace", "all", "", "", map[string]any{})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyWorkspaces].([]*workspacemodel.Workspace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
