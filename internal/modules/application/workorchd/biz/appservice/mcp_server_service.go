package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateMCPServerWithSession(ctx context.Context, session *RuntimeSession, cmd CreateMCPServerCommand) (ret MCPServer, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (MCPServer, error) {
		return contextregistryports.CreateMCPServer(ctx, hub, cmd)
	})
}

func CreateMCPServerWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateMCPServerCommand) (MCPServer, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (MCPServer, error) {
		return CreateMCPServerWithSession(ctx, session, cmd)
	})
}

func UpdateMCPServerWithSession(ctx context.Context, session *RuntimeSession, serverID string, cmd UpdateMCPServerCommand) (ret MCPServer, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (MCPServer, error) {
		return contextregistryports.UpdateMCPServerByID(ctx, hub, serverID, cmd)
	})
}

func UpdateMCPServerWithBackend(ctx context.Context, backend string, dataRoot string, serverID string, cmd UpdateMCPServerCommand) (MCPServer, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (MCPServer, error) {
		return UpdateMCPServerWithSession(ctx, session, serverID, cmd)
	})
}

func ReadMCPServerWithSession(ctx context.Context, session *RuntimeSession, serverID string) (ret MCPServer, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (MCPServer, error) {
		return contextregistryports.ReadMCPServerView(ctx, hub, serverID)
	})
}

func ReadMCPServerWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) (MCPServer, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (MCPServer, error) {
		return ReadMCPServerWithSession(ctx, session, serverID)
	})
}

func ListMCPServersWithSession(ctx context.Context, session *RuntimeSession) (ret []MCPServer, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]MCPServer, error) {
		return contextregistryports.ListMCPServers(ctx, hub)
	})
}

func ListMCPServersWithBackend(ctx context.Context, backend string, dataRoot string) ([]MCPServer, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]MCPServer, error) {
		return ListMCPServersWithSession(ctx, session)
	})
}
