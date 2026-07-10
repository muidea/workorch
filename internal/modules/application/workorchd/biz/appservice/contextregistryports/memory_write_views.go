package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateMemoryItemForWorkspace(ctx context.Context, commandHub magicevent.Hub, eventHub events.Hub, workspaceID string, cmd contextregistrycore.CreateMemoryItemCommand) (contextregistrycore.MemoryItem, error) {
	if commandHub == nil || eventHub == nil {
		return contextregistrycore.MemoryItem{}, common.ErrInvalidArgument
	}
	if err := EnsureWorkspaceExists(ctx, commandHub, workspaceID); err != nil {
		return contextregistrycore.MemoryItem{}, err
	}
	memoryID := cmd.MemoryID
	if memoryID == "" {
		memoryID = contextregistrycore.GenerateContextID("memory")
	}
	item, err := CreateMemoryItem(ctx, commandHub, eventHub, contextregistrycore.MemoryItemFromCommand(memoryID, workspaceID, cmd))
	if err != nil {
		return contextregistrycore.MemoryItem{}, err
	}
	return contextregistrycore.MapMemoryItem(item), nil
}

func UpdateMemoryItemView(ctx context.Context, commandHub magicevent.Hub, eventHub events.Hub, memoryID string, cmd contextregistrycore.UpdateMemoryItemCommand) (contextregistrycore.MemoryItem, error) {
	if commandHub == nil || eventHub == nil {
		return contextregistrycore.MemoryItem{}, common.ErrInvalidArgument
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return contextregistrycore.MemoryItem{}, common.ErrInvalidArgument
	}
	if strings.TrimSpace(cmd.MemoryID) != "" && strings.TrimSpace(cmd.MemoryID) != memoryID {
		return contextregistrycore.MemoryItem{}, common.ErrInvalidArgument
	}
	existing, err := ReadMemoryItem(ctx, commandHub, memoryID)
	if err != nil {
		return contextregistrycore.MemoryItem{}, err
	}
	merged := contextregistrycore.MergeMemoryUpdateCommand(existing, cmd)
	item, err := UpdateMemoryItem(ctx, commandHub, eventHub, contextregistrycore.MemoryItemFromMergedCommand(memoryID, existing.WorkspaceID, merged))
	if err != nil {
		return contextregistrycore.MemoryItem{}, err
	}
	return contextregistrycore.MapMemoryItem(item), nil
}

func DeleteMemoryItemView(ctx context.Context, commandHub magicevent.Hub, eventHub events.Hub, memoryID string) error {
	if commandHub == nil || eventHub == nil {
		return common.ErrInvalidArgument
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return common.ErrInvalidArgument
	}
	return DeleteMemoryItemWithLifecycleEvent(ctx, commandHub, eventHub, memoryID)
}
