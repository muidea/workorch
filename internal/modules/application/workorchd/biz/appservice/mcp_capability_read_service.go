package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadMCPCapabilityWithSession(ctx context.Context, session *RuntimeSession, capabilityID string) (ret MCPCapability, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (MCPCapability, error) {
		return contextregistryports.ReadMCPCapability(ctx, hub, capabilityID)
	})
}

func ReadMCPCapabilityWithBackend(ctx context.Context, backend string, dataRoot string, capabilityID string) (MCPCapability, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (MCPCapability, error) {
		return ReadMCPCapabilityWithSession(ctx, session, capabilityID)
	})
}

func ListMCPCapabilitiesWithSession(ctx context.Context, session *RuntimeSession, query MCPCapabilityQuery) (ret []MCPCapability, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]MCPCapability, error) {
		return contextregistryports.ListMCPCapabilities(ctx, hub, query)
	})
}

func ListMCPCapabilitiesWithBackend(ctx context.Context, backend string, dataRoot string, query MCPCapabilityQuery) ([]MCPCapability, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]MCPCapability, error) {
		return ListMCPCapabilitiesWithSession(ctx, session, query)
	})
}
