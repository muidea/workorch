package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateMCPServerWithSession(ctx context.Context, session *runtimecore.Session, cmd workorchmanagementcontract.MCPServer) (ret workorchmanagementcontract.MCPServer, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.MCPServer, error) {
		return contextregistryports.CreateMCPServer(ctx, hub, cmd)
	})
}

func CreateMCPServerWithBackend(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.MCPServer) (workorchmanagementcontract.MCPServer, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.MCPServer, error) {
		return CreateMCPServerWithSession(ctx, session, cmd)
	})
}

func UpdateMCPServerWithSession(ctx context.Context, session *runtimecore.Session, serverID string, cmd workorchmanagementcontract.MCPServer) (ret workorchmanagementcontract.MCPServer, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.MCPServer, error) {
		return contextregistryports.UpdateMCPServerByID(ctx, hub, serverID, cmd)
	})
}

func UpdateMCPServerWithBackend(ctx context.Context, backend string, dataRoot string, serverID string, cmd workorchmanagementcontract.MCPServer) (workorchmanagementcontract.MCPServer, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.MCPServer, error) {
		return UpdateMCPServerWithSession(ctx, session, serverID, cmd)
	})
}

func ReadMCPServerWithSession(ctx context.Context, session *runtimecore.Session, serverID string) (ret workorchmanagementcontract.MCPServer, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.MCPServer, error) {
		return contextregistryports.ReadMCPServerView(ctx, hub, serverID)
	})
}

func ReadMCPServerWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) (workorchmanagementcontract.MCPServer, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.MCPServer, error) {
		return ReadMCPServerWithSession(ctx, session, serverID)
	})
}

func ListMCPServersWithSession(ctx context.Context, session *runtimecore.Session) (ret []workorchmanagementcontract.MCPServer, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.MCPServer, error) {
		return contextregistryports.ListMCPServers(ctx, hub)
	})
}

func ListMCPServersWithBackend(ctx context.Context, backend string, dataRoot string) ([]workorchmanagementcontract.MCPServer, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.MCPServer, error) {
		return ListMCPServersWithSession(ctx, session)
	})
}
