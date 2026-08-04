package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ReadMCPCapabilityWithSession(ctx context.Context, session *runtimecore.Session, capabilityID string) (ret workorchmanagementcontract.MCPCapability, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.MCPCapability, error) {
		return contextregistryports.ReadMCPCapability(ctx, hub, capabilityID)
	})
}

func ReadMCPCapabilityWithBackend(ctx context.Context, backend string, dataRoot string, capabilityID string) (workorchmanagementcontract.MCPCapability, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.MCPCapability, error) {
		return ReadMCPCapabilityWithSession(ctx, session, capabilityID)
	})
}

func ListMCPCapabilitiesWithSession(ctx context.Context, session *runtimecore.Session, query contextregistrycore.MCPCapabilityQuery) (ret []workorchmanagementcontract.MCPCapability, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.MCPCapability, error) {
		return contextregistryports.ListMCPCapabilities(ctx, hub, query)
	})
}

func ListMCPCapabilitiesWithBackend(ctx context.Context, backend string, dataRoot string, query contextregistrycore.MCPCapabilityQuery) ([]workorchmanagementcontract.MCPCapability, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.MCPCapability, error) {
		return ListMCPCapabilitiesWithSession(ctx, session, query)
	})
}
