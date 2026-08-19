package contextregistryports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func PublishMemoryItem(ctx context.Context, hub magicevent.Hub, topic string, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	if hub == nil {
		return common.ErrInvalidArgument
	}
	switch topic {
	case memoryevents.TopicMemoryCommandCreate:
		event := magicevent.NewEventWithContext(topic, sourceModule, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.CreateMemoryCommandFromModel(item))
		ownerevent.BindLane(event, memoryevents.Contracts(), topic, "memory", string(item.ID), item.WorkspaceID, item.RunID, "")
		_, err := ownerevent.Memory[memoryevents.CreateMemoryResult](hub.Send(event), topic)
		return err
	case memoryevents.TopicMemoryCommandUpdate:
		event := magicevent.NewEventWithContext(topic, sourceModule, memoryevents.CommandDestination, magicevent.NewHeader(), ctx, memoryevents.UpdateMemoryCommand(memoryevents.CreateMemoryCommandFromModel(item)))
		ownerevent.BindLane(event, memoryevents.Contracts(), topic, "memory", string(item.ID), item.WorkspaceID, item.RunID, "")
		_, err := ownerevent.Memory[memoryevents.UpdateMemoryResult](hub.Send(event), topic)
		return err
	default:
		return common.ErrInvalidArgument
	}
}

func CreateMemoryItem(ctx context.Context, commandHub magicevent.Hub, item *memorymodel.MemoryItem) (*memorymodel.MemoryItem, error) {
	if err := PublishMemoryItem(ctx, commandHub, memoryevents.TopicMemoryCommandCreate, item); err != nil {
		return nil, err
	}
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	created, err := ReadMemoryItem(readCtx, commandHub, string(item.ID))
	if err != nil {
		return nil, err
	}
	return created, nil
}

func UpdateMemoryItem(ctx context.Context, commandHub magicevent.Hub, item *memorymodel.MemoryItem) (*memorymodel.MemoryItem, error) {
	if err := PublishMemoryItem(ctx, commandHub, memoryevents.TopicMemoryCommandUpdate, item); err != nil {
		return nil, err
	}
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	updated, err := ReadMemoryItem(readCtx, commandHub, string(item.ID))
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func DeleteMemoryItem(ctx context.Context, hub magicevent.Hub, memoryID string) error {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if hub == nil {
		return common.ErrInvalidArgument
	}
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(memoryevents.TopicMemoryCommandDelete, sourceModule, memoryevents.CommandDestination, magicevent.NewHeader(), commandCtx, memoryevents.DeleteMemoryCommand{WorkspaceID: workspaceID, MemoryID: common.MemoryID(memoryID)})
	ownerevent.BindLane(event, memoryevents.Contracts(), memoryevents.TopicMemoryCommandDelete, "memory", memoryID, workspaceID, "", "")
	_, err := ownerevent.Memory[memoryevents.DeleteMemoryResult](hub.Send(event), memoryevents.TopicMemoryCommandDelete)
	return err
}
