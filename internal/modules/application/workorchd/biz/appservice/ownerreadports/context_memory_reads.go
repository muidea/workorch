package ownerreadports

import (
	"context"

	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ReadMemoryItemByOwnerCommand(ctx context.Context, hub events.Hub, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := PublishCommandWithResponse(ctx, hub, memoryevents.TopicMemoryCommandGet, "memory", string(id), workspaceID, "", payloadWithWorkspaceScope(map[string]any{
		events.PayloadKeyMemoryID: string(id),
	}, workspaceID))
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyMemoryItem].(*memorymodel.MemoryItem)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ListMemoryItemsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, memoryevents.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func SearchMemoryByOwnerCommand(ctx context.Context, hub events.Hub, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, memoryevents.TopicMemoryCommandSearch, "memory_search", string(query.WorkspaceID), query.WorkspaceID, "", map[string]any{
		events.PayloadKeyMemorySearchQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
