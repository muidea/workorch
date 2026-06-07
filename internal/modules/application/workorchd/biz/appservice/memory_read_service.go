package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadMemoryItemWithSession(ctx context.Context, session *RuntimeSession, memoryID string) (ret MemoryItem, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (MemoryItem, error) {
		return contextregistryports.ReadMemoryItemView(ctx, hub, memoryID)
	})
}

func ReadMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, memoryID string) (MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (MemoryItem, error) {
		return ReadMemoryItemWithSession(ctx, session, memoryID)
	})
}

func ListMemoryByWorkspaceWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, query string) (ret []MemoryItem, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]MemoryItem, error) {
		return contextregistryports.ListMemoryByWorkspaceView(ctx, hub, workspaceID, query)
	})
}

func ListMemoryByWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, query string) ([]MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]MemoryItem, error) {
		return ListMemoryByWorkspaceWithSession(ctx, session, workspaceID, query)
	})
}

func SearchMemoryWithSession(ctx context.Context, session *RuntimeSession, query MemoryQuery) (ret []MemoryItem, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]MemoryItem, error) {
		return contextregistryports.SearchMemoryView(ctx, hub, query)
	})
}

func SearchMemoryWithBackend(ctx context.Context, backend string, dataRoot string, query MemoryQuery) ([]MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]MemoryItem, error) {
		return SearchMemoryWithSession(ctx, session, query)
	})
}
