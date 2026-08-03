package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreateMCPCapability(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.MCPCapability) (contextregistrycore.MCPCapability, error) {
	if hub == nil {
		return contextregistrycore.MCPCapability{}, common.ErrInvalidArgument
	}
	command, err := mcpCapabilityCommandFromView(cmd)
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandCreateCapability, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandCreateCapability, "mcp_capability", cmd.ID, "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandCreateCapability)
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	return mcpCapabilityFromCommandResult(result)
}

func UpdateMCPCapability(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.MCPCapability) (contextregistrycore.MCPCapability, error) {
	if hub == nil {
		return contextregistrycore.MCPCapability{}, common.ErrInvalidArgument
	}
	command, err := mcpCapabilityCommandFromView(cmd)
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandUpdateCapability, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.UpdateCapabilityCommand(command))
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandUpdateCapability, "mcp_capability", cmd.ID, "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandUpdateCapability)
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
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandGetCapability, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.GetCapabilityCommand{CapabilityID: common.MCPCapabilityID(capabilityID)})
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandGetCapability, "mcp_capability", capabilityID, "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandGetCapability)
	if err != nil {
		return contextregistrycore.MCPCapability{}, err
	}
	return mcpCapabilityFromCommandResult(result)
}

func ListMCPCapabilities(ctx context.Context, hub magicevent.Hub, query contextregistrycore.MCPCapabilityQuery) ([]contextregistrycore.MCPCapability, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandListCapabilities, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ListCapabilitiesCommand{Query: mcpCapabilityQueryFromView(query)})
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandListCapabilities, "mcp_capability", "all", "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandListCapabilities)
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
