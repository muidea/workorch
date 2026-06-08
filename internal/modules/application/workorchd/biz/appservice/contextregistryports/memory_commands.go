package contextregistryports

import (
	"context"

	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func PublishMemoryItem(ctx context.Context, hub events.Hub, topic string, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	return PublishCommand(ctx, hub, topic, "memory", string(item.ID), map[string]any{
		events.PayloadKeyCommand: memoryCommandFromModel(item),
	})
}

func CreateMemoryItem(ctx context.Context, hub events.Hub, item *memorymodel.MemoryItem) (*memorymodel.MemoryItem, error) {
	if err := PublishMemoryItem(ctx, hub, memoryevents.TopicMemoryCommandCreate, item); err != nil {
		return nil, err
	}
	created, err := ReadMemoryItem(ctx, hub, string(item.ID))
	if err != nil {
		return nil, err
	}
	if err := PublishMemoryEvent(ctx, hub, memoryevents.TopicMemoryCreated, created); err != nil {
		return nil, err
	}
	return created, nil
}

func UpdateMemoryItem(ctx context.Context, hub events.Hub, item *memorymodel.MemoryItem) (*memorymodel.MemoryItem, error) {
	if err := PublishMemoryItem(ctx, hub, memoryevents.TopicMemoryCommandUpdate, item); err != nil {
		return nil, err
	}
	updated, err := ReadMemoryItem(ctx, hub, string(item.ID))
	if err != nil {
		return nil, err
	}
	if err := PublishMemoryEvent(ctx, hub, memoryevents.TopicMemoryUpdated, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

func DeleteMemoryItem(ctx context.Context, hub events.Hub, memoryID string) error {
	return PublishCommand(ctx, hub, memoryevents.TopicMemoryCommandDelete, "memory", memoryID, map[string]any{
		events.PayloadKeyCommand: memorycontracts.DeleteMemoryItemCommand{MemoryID: common.MemoryID(memoryID)},
	})
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
