package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateMemoryItemForWorkspace(ctx context.Context, commandHub magicevent.Hub, workspaceID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
	if commandHub == nil {
		return workorchmanagementcontract.MemoryItem{}, common.ErrInvalidArgument
	}
	if err := EnsureWorkspaceExists(ctx, commandHub, workspaceID); err != nil {
		return workorchmanagementcontract.MemoryItem{}, err
	}
	memoryID := cmd.MemoryID
	if memoryID == "" {
		memoryID = contextregistrycore.GenerateContextID("memory")
	}
	item, err := CreateMemoryItem(ctx, commandHub, contextregistrycore.MemoryItemFromCommand(memoryID, workspaceID, cmd))
	if err != nil {
		return workorchmanagementcontract.MemoryItem{}, err
	}
	return contextregistrycore.MapMemoryItem(item), nil
}

func UpdateMemoryItemView(ctx context.Context, commandHub magicevent.Hub, memoryID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
	if commandHub == nil {
		return workorchmanagementcontract.MemoryItem{}, common.ErrInvalidArgument
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return workorchmanagementcontract.MemoryItem{}, common.ErrInvalidArgument
	}
	if strings.TrimSpace(cmd.MemoryID) != "" && strings.TrimSpace(cmd.MemoryID) != memoryID {
		return workorchmanagementcontract.MemoryItem{}, common.ErrInvalidArgument
	}
	existing, err := ReadMemoryItem(ctx, commandHub, memoryID)
	if err != nil {
		return workorchmanagementcontract.MemoryItem{}, err
	}
	merged := contextregistrycore.MergeMemoryUpdateCommand(existing, cmd)
	item, err := UpdateMemoryItem(ctx, commandHub, contextregistrycore.MemoryItemFromMergedCommand(memoryID, existing.WorkspaceID, merged))
	if err != nil {
		return workorchmanagementcontract.MemoryItem{}, err
	}
	return contextregistrycore.MapMemoryItem(item), nil
}

func DeleteMemoryItemView(ctx context.Context, commandHub magicevent.Hub, memoryID string) error {
	if commandHub == nil {
		return common.ErrInvalidArgument
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return common.ErrInvalidArgument
	}
	existing, err := ReadMemoryItem(ctx, commandHub, memoryID)
	if err != nil {
		return err
	}
	return DeleteMemoryItem(common.ContextWithRuntimeWorkspaceID(ctx, existing.WorkspaceID), commandHub, memoryID)
}
