package ownerreadports

import (
	"context"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadDoDByOwnerCommand(ctx context.Context, hub events.Hub, dodID common.DoDID) (*workspacemodel.DoD, error) {
	if hub == nil || dodID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandGetDoD, "dod", string(dodID), "", "", map[string]any{
		events.PayloadKeyDoDID: string(dodID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyDoD].(*workspacemodel.DoD)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ListDoDsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandListDoDsByWS, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyDoDs].([]*workspacemodel.DoD)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
