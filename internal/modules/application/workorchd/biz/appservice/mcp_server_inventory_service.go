package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ListMCPServerToolsWithSession(ctx context.Context, session *runtimecore.Session, serverID string) (ret []workorchmanagementcontract.MCPTool, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.MCPTool, error) {
		return contextregistryports.ListMCPServerTools(ctx, hub, serverID)
	})
}

func ListMCPServerToolsWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) ([]workorchmanagementcontract.MCPTool, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.MCPTool, error) {
		return ListMCPServerToolsWithSession(ctx, session, serverID)
	})
}

func ListMCPServerResourcesWithSession(ctx context.Context, session *runtimecore.Session, serverID string) (ret []workorchmanagementcontract.MCPResource, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.MCPResource, error) {
		return contextregistryports.ListMCPServerResources(ctx, hub, serverID)
	})
}

func ListMCPServerResourcesWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) ([]workorchmanagementcontract.MCPResource, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.MCPResource, error) {
		return ListMCPServerResourcesWithSession(ctx, session, serverID)
	})
}

func ListMCPServerPromptsWithSession(ctx context.Context, session *runtimecore.Session, serverID string) (ret []workorchmanagementcontract.MCPPrompt, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.MCPPrompt, error) {
		return contextregistryports.ListMCPServerPrompts(ctx, hub, serverID)
	})
}

func ListMCPServerPromptsWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) ([]workorchmanagementcontract.MCPPrompt, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.MCPPrompt, error) {
		return ListMCPServerPromptsWithSession(ctx, session, serverID)
	})
}
