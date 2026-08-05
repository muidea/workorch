package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateMCPCapabilityWithSession(ctx context.Context, session *runtimecore.Session, cmd workorchmanagementcontract.MCPCapabilityMutation) (ret workorchmanagementcontract.MCPCapability, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.MCPCapability, error) {
		return contextregistryports.CreateMCPCapability(ctx, hub, cmd)
	})
}

func CreateMCPCapabilityWithBackend(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.MCPCapabilityMutation) (workorchmanagementcontract.MCPCapability, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.MCPCapability, error) {
		return CreateMCPCapabilityWithSession(ctx, session, cmd)
	})
}

func UpdateMCPCapabilityWithSession(ctx context.Context, session *runtimecore.Session, capabilityID string, cmd workorchmanagementcontract.MCPCapabilityMutation) (ret workorchmanagementcontract.MCPCapability, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.MCPCapability, error) {
		return contextregistryports.UpdateMCPCapabilityByID(ctx, hub, capabilityID, cmd)
	})
}

func UpdateMCPCapabilityWithBackend(ctx context.Context, backend string, dataRoot string, capabilityID string, cmd workorchmanagementcontract.MCPCapabilityMutation) (workorchmanagementcontract.MCPCapability, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.MCPCapability, error) {
		return UpdateMCPCapabilityWithSession(ctx, session, capabilityID, cmd)
	})
}
