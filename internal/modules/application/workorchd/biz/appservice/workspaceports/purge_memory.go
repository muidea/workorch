package workspaceports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
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
		commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandDelete, sourceModule, memoryevents.CommandDestination, magicevent.NewHeader(), commandCtx, memoryevents.DeleteMemoryCommand{WorkspaceID: workspaceID, MemoryID: item.ID})
		ownerevent.BindLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandDelete, "memory", string(item.ID), workspaceID, item.RunID, "")
		if _, err := ownerevent.Memory(hub.Send(event), memoryevents.TopicMemoryCommandDelete); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}
