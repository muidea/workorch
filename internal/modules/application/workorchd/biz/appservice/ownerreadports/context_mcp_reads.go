package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadMCPServerByOwnerCommand(ctx context.Context, hub magicevent.Hub, id common.MCPServerID) (*mcpmodel.Server, error) {
	if hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandGetServer, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.GetServerCommand{ServerID: id})
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandGetServer, "mcp_server", string(id), "", "", "")
	result, err := ownerevent.MCP[mcpevents.GetServerResult](hub.Send(event), mcpevents.TopicMCPCommandGetServer)
	if err != nil {
		return nil, err
	}
	if result.Server == nil {
		return nil, common.ErrInvalidArgument
	}
	return result.Server.Model()
}

func ReadMCPCapabilityByOwnerCommand(ctx context.Context, hub magicevent.Hub, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	if hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandGetCapability, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.GetCapabilityCommand{CapabilityID: id})
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandGetCapability, "mcp_capability", string(id), "", "", "")
	result, err := ownerevent.MCP[mcpevents.GetCapabilityResult](hub.Send(event), mcpevents.TopicMCPCommandGetCapability)
	if err != nil {
		return nil, err
	}
	if result.Capability == nil {
		return nil, common.ErrInvalidArgument
	}
	return mcpmodel.CloneCapability(result.Capability), nil
}

func ReadMCPSelectionSnapshot(ctx context.Context, hub magicevent.Hub, serverIDs []string, capabilityIDs []string) ([]*mcpmodel.Server, []*mcpmodel.Capability, string, error) {
	if hub == nil {
		return nil, nil, "", common.ErrInvalidArgument
	}
	command := mcpevents.ReadSelectionSnapshotCommand{
		ServerIDs:     make([]common.MCPServerID, 0, len(serverIDs)),
		CapabilityIDs: make([]common.MCPCapabilityID, 0, len(capabilityIDs)),
	}
	for _, id := range serverIDs {
		command.ServerIDs = append(command.ServerIDs, common.MCPServerID(id))
	}
	for _, id := range capabilityIDs {
		command.CapabilityIDs = append(command.CapabilityIDs, common.MCPCapabilityID(id))
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandReadSelectionSnapshot, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandReadSelectionSnapshot, "mcp_registry", "selection", "", "", "")
	result, err := ownerevent.MCP[mcpevents.SelectionSnapshotResult](hub.Send(event), mcpevents.TopicMCPCommandReadSelectionSnapshot)
	if err != nil {
		return nil, nil, "", err
	}
	servers := make([]*mcpmodel.Server, 0, len(result.Servers))
	for _, item := range result.Servers {
		server, convertErr := item.Model()
		if convertErr != nil {
			return nil, nil, "", convertErr
		}
		servers = append(servers, server)
	}
	return servers, mcpmodel.CloneCapabilities(result.Capabilities), result.RegistryRevision, nil
}
