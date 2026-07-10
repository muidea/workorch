package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"

	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadMCPServerByOwnerCommand(ctx context.Context, hub magicevent.Hub, id common.MCPServerID) (*mcpmodel.Server, error) {
	if hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[mcpevents.GetServerCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandGetServer, mcpevents.CommandDestination, "mcp_server", string(id), mcpevents.GetServerCommand{ServerID: id})
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
	result, err := sendMagicCommand[mcpevents.GetCapabilityCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandGetCapability, mcpevents.CommandDestination, "mcp_capability", string(id), mcpevents.GetCapabilityCommand{CapabilityID: id})
	if err != nil {
		return nil, err
	}
	if result.Capability == nil {
		return nil, common.ErrInvalidArgument
	}
	return result.Capability, nil
}
