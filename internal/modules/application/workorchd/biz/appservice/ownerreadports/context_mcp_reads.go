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
	ownerevent.BindLane(event, mcpevents.TopicMCPCommandGetServer, "mcp_server", string(id), "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandGetServer)
	if err != nil {
		return nil, err
	}
	if result.Server == nil {
		return nil, common.ErrInvalidArgument
	}
	return result.Server, nil
}

func ReadMCPCapabilityByOwnerCommand(ctx context.Context, hub magicevent.Hub, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	if hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandGetCapability, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.GetCapabilityCommand{CapabilityID: id})
	ownerevent.BindLane(event, mcpevents.TopicMCPCommandGetCapability, "mcp_capability", string(id), "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandGetCapability)
	if err != nil {
		return nil, err
	}
	if result.Capability == nil {
		return nil, common.ErrInvalidArgument
	}
	return result.Capability, nil
}
