package contextregistryports

import (
	"context"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
)

const memoryEventSourceModule = "workorchd/appservice"

func DeleteMemoryItemWithLifecycleEvent(ctx context.Context, hub events.Hub, memoryID string) error {
	item, err := ReadMemoryItem(ctx, hub, memoryID)
	if err != nil {
		return err
	}
	if err := DeleteMemoryItem(ctx, hub, memoryID); err != nil {
		return err
	}
	return PublishMemoryEvent(ctx, hub, events.TopicMemoryDeleted, item)
}

func PublishMemoryEvent(ctx context.Context, hub events.Hub, topic string, item *memorymodel.MemoryItem) error {
	if hub == nil || item == nil {
		return nil
	}
	return hub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: memoryEventSourceModule,
		SubjectType:  "memory",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload:      events.MemoryPayload(item.MemoryType, item.Summary, item.Content, item.SourceType, item.SourceRef, item.Tags, item.Importance, item.Confidence, item.Visibility),
	})
}
