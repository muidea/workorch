package workspaceports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func purgeWorkspaceMemoryItems(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
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
		if _, err := sendWorkspaceMagicCommand[memoryevents.DeleteMemoryCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, memoryevents.TopicMemoryCommandDelete, memoryevents.CommandDestination, "memory", string(item.ID), workspaceID, item.RunID, memoryevents.DeleteMemoryCommand{WorkspaceID: workspaceID, MemoryID: item.ID}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
