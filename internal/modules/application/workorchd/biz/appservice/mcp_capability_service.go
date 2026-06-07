package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateMCPCapabilityWithSession(ctx context.Context, session *RuntimeSession, cmd CreateMCPCapabilityCommand) (ret MCPCapability, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (MCPCapability, error) {
		return contextregistryports.CreateMCPCapability(ctx, hub, cmd)
	})
}

func CreateMCPCapabilityWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateMCPCapabilityCommand) (MCPCapability, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (MCPCapability, error) {
		return CreateMCPCapabilityWithSession(ctx, session, cmd)
	})
}

func UpdateMCPCapabilityWithSession(ctx context.Context, session *RuntimeSession, capabilityID string, cmd UpdateMCPCapabilityCommand) (ret MCPCapability, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (MCPCapability, error) {
		return contextregistryports.UpdateMCPCapabilityByID(ctx, hub, capabilityID, cmd)
	})
}

func UpdateMCPCapabilityWithBackend(ctx context.Context, backend string, dataRoot string, capabilityID string, cmd UpdateMCPCapabilityCommand) (MCPCapability, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (MCPCapability, error) {
		return UpdateMCPCapabilityWithSession(ctx, session, capabilityID, cmd)
	})
}
