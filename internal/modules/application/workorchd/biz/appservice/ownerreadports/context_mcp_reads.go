package ownerreadports

import (
	"context"

	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"

	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadMCPServerByOwnerCommand(ctx context.Context, hub events.Hub, id common.MCPServerID) (*mcpmodel.Server, error) {
	if hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, mcpevents.TopicMCPCommandGetServer, "mcp_server", string(id), "", "", map[string]any{
		events.PayloadKeyServerID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyServer].(*mcpmodel.Server)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ReadMCPCapabilityByOwnerCommand(ctx context.Context, hub events.Hub, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	if hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, mcpevents.TopicMCPCommandGetCapability, "mcp_capability", string(id), "", "", map[string]any{
		events.PayloadKeyCapabilityID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyCapability].(*mcpmodel.Capability)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}
