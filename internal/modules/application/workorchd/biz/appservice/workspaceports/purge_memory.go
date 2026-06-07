package workspaceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func purgeWorkspaceMemoryItems(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	response, err := ownercommandports.PublishWithResponse(ctx, hub, events.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := ownercommandports.Publish(ctx, hub, events.TopicMemoryCommandDelete, "memory", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyCommand: memorycontracts.DeleteMemoryItemCommand{MemoryID: item.ID},
		}); err != nil {
			return err
		}
	}
	return nil
}
