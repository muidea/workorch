package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
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
	response, err := ownerreadports.PublishCommandWithResponse(ctx, r.Hub, events.TopicMCPCommandListCapabilities, "mcp_capability", "all", "", "", map[string]any{})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyCapabilities].([]*mcpmodel.Capability)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
