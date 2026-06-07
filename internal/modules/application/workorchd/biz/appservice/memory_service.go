package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/events"
)

type MemoryItem = contextregistrycore.MemoryItem

type CreateMemoryItemCommand = contextregistrycore.CreateMemoryItemCommand
type UpdateMemoryItemCommand = CreateMemoryItemCommand
type MemoryQuery = contextregistrycore.MemoryQuery

func CreateMemoryItemWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd CreateMemoryItemCommand) (ret MemoryItem, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (MemoryItem, error) {
		return contextregistryports.CreateMemoryItemForWorkspace(ctx, hub, workspaceID, cmd)
	})
}

func CreateMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd CreateMemoryItemCommand) (MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (MemoryItem, error) {
		return CreateMemoryItemWithSession(ctx, session, workspaceID, cmd)
	})
}

func UpdateMemoryItemWithSession(ctx context.Context, session *RuntimeSession, memoryID string, cmd UpdateMemoryItemCommand) (ret MemoryItem, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (MemoryItem, error) {
		return contextregistryports.UpdateMemoryItemView(ctx, hub, memoryID, cmd)
	})
}

func UpdateMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, memoryID string, cmd UpdateMemoryItemCommand) (MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (MemoryItem, error) {
		return UpdateMemoryItemWithSession(ctx, session, memoryID, cmd)
	})
}

func DeleteMemoryItemWithSession(ctx context.Context, session *RuntimeSession, memoryID string) (err error) {
	_, err = withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (struct{}, error) {
		return struct{}{}, contextregistryports.DeleteMemoryItemView(ctx, hub, memoryID)
	})
	return err
}

func DeleteMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, memoryID string) error {
	_, err := withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (struct{}, error) {
		return struct{}{}, DeleteMemoryItemWithSession(ctx, session, memoryID)
	})
	return err
}
