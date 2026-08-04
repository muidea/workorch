package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ReadMemoryItemWithSession(ctx context.Context, session *runtimecore.Session, memoryID string) (ret workorchmanagementcontract.MemoryItem, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.MemoryItem, error) {
		return contextregistryports.ReadMemoryItemView(ctx, hub, memoryID)
	})
}

func ReadMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, memoryID string) (workorchmanagementcontract.MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.MemoryItem, error) {
		return ReadMemoryItemWithSession(ctx, session, memoryID)
	})
}

func ListMemoryByWorkspaceWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, query string) (ret []workorchmanagementcontract.MemoryItem, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.MemoryItem, error) {
		return contextregistryports.ListMemoryByWorkspaceView(ctx, hub, workspaceID, query)
	})
}

func ListMemoryByWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, query string) ([]workorchmanagementcontract.MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.MemoryItem, error) {
		return ListMemoryByWorkspaceWithSession(ctx, session, workspaceID, query)
	})
}

func SearchMemoryWithSession(ctx context.Context, session *runtimecore.Session, query workorchmanagementcontract.MemorySearchQuery) (ret []workorchmanagementcontract.MemoryItem, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.MemoryItem, error) {
		return contextregistryports.SearchMemoryView(ctx, hub, query)
	})
}

func SearchMemoryWithBackend(ctx context.Context, backend string, dataRoot string, query workorchmanagementcontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.MemoryItem, error) {
		return SearchMemoryWithSession(ctx, session, query)
	})
}
