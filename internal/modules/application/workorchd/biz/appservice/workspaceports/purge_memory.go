package workspaceports

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func purgeWorkspaceMemoryItems(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	items, err := ownerreadports.ListMemoryItemsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if _, err := sendWorkspaceMagicCommand[memoryevents.DeleteMemoryCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, memoryevents.TopicMemoryCommandDelete, memoryevents.CommandDestination, "memory", string(item.ID), workspaceID, item.RunID, memoryevents.DeleteMemoryCommand{MemoryID: item.ID}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
