package contextregistryports

import (
	"context"
	"strings"

	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateMCPCapability(ctx context.Context, hub events.Hub, cmd contextregistrycore.MCPCapability) (contextregistrycore.MCPCapability, error) {
	response, err := PublishCommandWithResponse(ctx, hub, mcpevents.TopicMCPCommandCreateCapability, "mcp_capability", cmd.ID, map[string]any{
		events.PayloadKeyCommand: mcpCapabilityCommandFromView(cmd),
	})
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	return contextregistrycore.MCPCapabilityFromPayload(response.Payload, events.PayloadKeyCapability)
}

func UpdateMCPCapability(ctx context.Context, hub events.Hub, cmd contextregistrycore.MCPCapability) (contextregistrycore.MCPCapability, error) {
	response, err := PublishCommandWithResponse(ctx, hub, mcpevents.TopicMCPCommandUpdateCapability, "mcp_capability", cmd.ID, map[string]any{
		events.PayloadKeyCommand: mcpCapabilityCommandFromView(cmd),
	})
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	return contextregistrycore.MCPCapabilityFromPayload(response.Payload, events.PayloadKeyCapability)
}

func UpdateMCPCapabilityByID(ctx context.Context, hub events.Hub, capabilityID string, cmd contextregistrycore.MCPCapability) (contextregistrycore.MCPCapability, error) {
	capabilityID = strings.TrimSpace(capabilityID)
	cmd.ID = strings.TrimSpace(cmd.ID)
	if capabilityID == "" {
		return contextregistrycore.MCPCapability{}, common.ErrInvalidArgument
	}
	if cmd.ID != "" && cmd.ID != capabilityID {
		return contextregistrycore.MCPCapability{}, common.ErrInvalidArgument
	}
	cmd.ID = capabilityID
	return UpdateMCPCapability(ctx, hub, cmd)
}

func ReadMCPCapability(ctx context.Context, hub events.Hub, capabilityID string) (contextregistrycore.MCPCapability, error) {
	response, err := PublishCommandWithResponse(ctx, hub, mcpevents.TopicMCPCommandGetCapability, "mcp_capability", capabilityID, map[string]any{
		events.PayloadKeyCapabilityID: capabilityID,
	})
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	return contextregistrycore.MCPCapabilityFromPayload(response.Payload, events.PayloadKeyCapability)
}

func ListMCPCapabilities(ctx context.Context, hub events.Hub, query contextregistrycore.MCPCapabilityQuery) ([]contextregistrycore.MCPCapability, error) {
	response, err := PublishCommandWithResponse(ctx, hub, mcpevents.TopicMCPCommandListCapabilities, "mcp_capability", "all", map[string]any{
		events.PayloadKeyCapabilityQuery: mcpCapabilityQueryFromView(query),
	})
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MCPCapabilitiesFromPayload(response.Payload, events.PayloadKeyCapabilities)
}
