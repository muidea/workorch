package contextregistryports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
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
	payload := map[string]any{
		events.PayloadKeyCommand: memoryCommandFromModel(item),
	}
	if item.WorkspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(item.WorkspaceID)
	}
	if item.RunID != "" {
		payload[events.PayloadKeyRunID] = string(item.RunID)
	}
	return ownercommandports.PublishFrom(ctx, sourceModule, hub, topic, "memory", string(item.ID), item.WorkspaceID, item.RunID, payload)
}

func CreateMemoryItem(ctx context.Context, hub events.Hub, item *memorymodel.MemoryItem) (*memorymodel.MemoryItem, error) {
	if err := PublishMemoryItem(ctx, hub, memoryevents.TopicMemoryCommandCreate, item); err != nil {
		return nil, err
	}
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	created, err := ReadMemoryItem(readCtx, hub, string(item.ID))
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
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	updated, err := ReadMemoryItem(readCtx, hub, string(item.ID))
	if err != nil {
		return nil, err
	}
	if err := PublishMemoryEvent(ctx, hub, memoryevents.TopicMemoryUpdated, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

func DeleteMemoryItem(ctx context.Context, hub events.Hub, memoryID string) error {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyCommand: memorycontracts.DeleteMemoryItemCommand{MemoryID: common.MemoryID(memoryID)},
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	return ownercommandports.PublishFrom(ctx, sourceModule, hub, memoryevents.TopicMemoryCommandDelete, "memory", memoryID, workspaceID, "", payload)
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
