package contextregistryports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func PublishMemoryItem(ctx context.Context, hub magicevent.Hub, topic string, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	switch topic {
	case memoryevents.TopicMemoryCommandCreate:
		_, err := sendMagicCommand[memoryevents.CreateMemoryCommand, memoryevents.MemoryCommandResult](ctx, hub, topic, memoryevents.CommandDestination, "memory", string(item.ID), memoryevents.CreateMemoryCommand(memoryCommandFromModel(item)))
		return err
	case memoryevents.TopicMemoryCommandUpdate:
		_, err := sendMagicCommand[memoryevents.UpdateMemoryCommand, memoryevents.MemoryCommandResult](ctx, hub, topic, memoryevents.CommandDestination, "memory", string(item.ID), memoryevents.UpdateMemoryCommand(memoryCommandFromModel(item)))
		return err
	default:
		return common.ErrInvalidArgument
	}
}

func CreateMemoryItem(ctx context.Context, commandHub magicevent.Hub, eventHub events.Hub, item *memorymodel.MemoryItem) (*memorymodel.MemoryItem, error) {
	if err := PublishMemoryItem(ctx, commandHub, memoryevents.TopicMemoryCommandCreate, item); err != nil {
		return nil, err
	}
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	created, err := ReadMemoryItem(readCtx, commandHub, string(item.ID))
	if err != nil {
		return nil, err
	}
	if err := PublishMemoryEvent(ctx, eventHub, memoryevents.TopicMemoryCreated, created); err != nil {
		return nil, err
	}
	return created, nil
}

func UpdateMemoryItem(ctx context.Context, commandHub magicevent.Hub, eventHub events.Hub, item *memorymodel.MemoryItem) (*memorymodel.MemoryItem, error) {
	if err := PublishMemoryItem(ctx, commandHub, memoryevents.TopicMemoryCommandUpdate, item); err != nil {
		return nil, err
	}
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	updated, err := ReadMemoryItem(readCtx, commandHub, string(item.ID))
	if err != nil {
		return nil, err
	}
	if err := PublishMemoryEvent(ctx, eventHub, memoryevents.TopicMemoryUpdated, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

func DeleteMemoryItem(ctx context.Context, hub magicevent.Hub, memoryID string) error {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	_, err := sendMagicCommand[memoryevents.DeleteMemoryCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, memoryevents.TopicMemoryCommandDelete, memoryevents.CommandDestination, "memory", memoryID, memoryevents.DeleteMemoryCommand{MemoryID: common.MemoryID(memoryID)})
	return err
}

func memoryCommandFromModel(item *memorymodel.MemoryItem) memorycontracts.CreateMemoryItemCommand {
	if item == nil {
		return memorycontracts.CreateMemoryItemCommand{}
	}
	return memorycontracts.CreateMemoryItemCommand{
		MemoryID:            item.ID,
		WorkspaceID:         item.WorkspaceID,
		RunID:               item.RunID,
		SourceType:          item.SourceType,
		SourceRef:           item.SourceRef,
		MemoryType:          item.MemoryType,
		Content:             item.Content,
		Summary:             item.Summary,
		Tags:                append([]string(nil), item.Tags...),
		Importance:          item.Importance,
		Confidence:          item.Confidence,
		Visibility:          item.Visibility,
		EmbeddingProviderID: item.EmbeddingProviderID,
		EmbeddingModel:      item.EmbeddingModel,
		EmbeddingText:       item.EmbeddingText,
		Vector:              append([]float64(nil), item.Vector...),
	}
}
