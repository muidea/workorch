package workspaceports

import (
	"context"
	"errors"

	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func purgeWorkspaceMemoryItems(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	response, err := ownerreadports.PublishCommandWithResponse(ctx, hub, memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
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
		if err := ownercommandports.Publish(ctx, hub, memoryevents.TopicMemoryCommandDelete, "memory", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyCommand: memorycontracts.DeleteMemoryItemCommand{MemoryID: item.ID},
		}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
