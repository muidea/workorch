package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListMCPServerToolsWithSession(ctx context.Context, session *RuntimeSession, serverID string) (ret []MCPTool, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]MCPTool, error) {
		return contextregistryports.ListMCPServerTools(ctx, hub, serverID)
	})
}

func ListMCPServerToolsWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) ([]MCPTool, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]MCPTool, error) {
		return ListMCPServerToolsWithSession(ctx, session, serverID)
	})
}

func ListMCPServerResourcesWithSession(ctx context.Context, session *RuntimeSession, serverID string) (ret []MCPResource, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]MCPResource, error) {
		return contextregistryports.ListMCPServerResources(ctx, hub, serverID)
	})
}

func ListMCPServerResourcesWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) ([]MCPResource, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]MCPResource, error) {
		return ListMCPServerResourcesWithSession(ctx, session, serverID)
	})
}

func ListMCPServerPromptsWithSession(ctx context.Context, session *RuntimeSession, serverID string) (ret []MCPPrompt, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]MCPPrompt, error) {
		return contextregistryports.ListMCPServerPrompts(ctx, hub, serverID)
	})
}

func ListMCPServerPromptsWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) ([]MCPPrompt, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]MCPPrompt, error) {
		return ListMCPServerPromptsWithSession(ctx, session, serverID)
	})
}
