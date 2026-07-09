package readmodelports

import (
	"context"

	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type MCPCapabilityReader struct {
	Hub events.Hub
}

func (r MCPCapabilityReader) List(ctx context.Context) ([]*mcpmodel.Capability, error) {
	if r.Hub == nil {
		return nil, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[mcpevents.ListCapabilitiesCommand, mcpevents.MCPCommandResult](ctx, r.Hub, mcpevents.TopicMCPCommandListCapabilities, mcpevents.CommandDestination, "application/workorchd/readmodel", "mcp_capability", "all", mcpevents.ListCapabilitiesCommand{})
	if err != nil {
		return nil, err
	}
	return result.Capabilities, nil
}
