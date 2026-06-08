package contextregistryports

import (
	"context"

	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ReadMemoryItem(ctx context.Context, hub events.Hub, memoryID string) (*memorymodel.MemoryItem, error) {
	response, err := PublishCommandWithResponse(ctx, hub, memoryevents.TopicMemoryCommandGet, "memory", memoryID, map[string]any{
		events.PayloadKeyMemoryID: memoryID,
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyMemoryItem].(*memorymodel.MemoryItem)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ListMemoryByWorkspace(ctx context.Context, hub events.Hub, workspaceID string) ([]*memorymodel.MemoryItem, error) {
	response, err := PublishCommandWithResponse(ctx, hub, memoryevents.TopicMemoryCommandListByWorkspace, "workspace", workspaceID, map[string]any{
		events.PayloadKeyWorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MemoryItemsFromPayload(response.Payload, events.PayloadKeyMemoryItems)
}

func SearchMemory(ctx context.Context, hub events.Hub, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	response, err := PublishCommandWithResponse(ctx, hub, memoryevents.TopicMemoryCommandSearch, "memory", "search", map[string]any{
		events.PayloadKeyMemorySearchQuery: query,
	})
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MemoryItemsFromPayload(response.Payload, events.PayloadKeyMemoryItems)
}
