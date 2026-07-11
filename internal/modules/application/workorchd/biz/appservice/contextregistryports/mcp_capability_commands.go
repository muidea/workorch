package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreateMCPCapability(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.MCPCapability) (contextregistrycore.MCPCapability, error) {
	if hub == nil {
		return contextregistrycore.MCPCapability{}, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[mcpevents.CreateCapabilityCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandCreateCapability, mcpevents.CommandDestination, "mcp_capability", cmd.ID, mcpevents.CreateCapabilityCommand(mcpCapabilityCommandFromView(cmd)))
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	return mcpCapabilityFromCommandResult(result)
}

func UpdateMCPCapability(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.MCPCapability) (contextregistrycore.MCPCapability, error) {
	if hub == nil {
		return contextregistrycore.MCPCapability{}, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[mcpevents.UpdateCapabilityCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandUpdateCapability, mcpevents.CommandDestination, "mcp_capability", cmd.ID, mcpevents.UpdateCapabilityCommand(mcpCapabilityCommandFromView(cmd)))
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	return mcpCapabilityFromCommandResult(result)
}

func UpdateMCPCapabilityByID(ctx context.Context, hub magicevent.Hub, capabilityID string, cmd contextregistrycore.MCPCapability) (contextregistrycore.MCPCapability, error) {
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

func ReadMCPCapability(ctx context.Context, hub magicevent.Hub, capabilityID string) (contextregistrycore.MCPCapability, error) {
	if hub == nil {
		return contextregistrycore.MCPCapability{}, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[mcpevents.GetCapabilityCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandGetCapability, mcpevents.CommandDestination, "mcp_capability", capabilityID, mcpevents.GetCapabilityCommand{CapabilityID: common.MCPCapabilityID(capabilityID)})
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	return mcpCapabilityFromCommandResult(result)
}

func ListMCPCapabilities(ctx context.Context, hub magicevent.Hub, query contextregistrycore.MCPCapabilityQuery) ([]contextregistrycore.MCPCapability, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[mcpevents.ListCapabilitiesCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandListCapabilities, mcpevents.CommandDestination, "mcp_capability", "all", mcpevents.ListCapabilitiesCommand{Query: mcpCapabilityQueryFromView(query)})
	if err != nil {
		return nil, err
	}
	return mcpCapabilitiesFromCommandResult(result)
}

func mcpCapabilityFromCommandResult(result mcpevents.MCPCommandResult) (contextregistrycore.MCPCapability, error) {
	if result.Capability == nil {
		return contextregistrycore.MCPCapability{}, common.ErrInvalidArgument
	}
	return contextregistrycore.MapMCPCapability(result.Capability), nil
}

func mcpCapabilitiesFromCommandResult(result mcpevents.MCPCommandResult) ([]contextregistrycore.MCPCapability, error) {
	ret := make([]contextregistrycore.MCPCapability, 0, len(result.Capabilities))
	for _, item := range result.Capabilities {
		ret = append(ret, contextregistrycore.MapMCPCapability(item))
	}
	return ret, nil
}
