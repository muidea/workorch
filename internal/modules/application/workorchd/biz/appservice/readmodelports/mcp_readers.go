package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MCPCapabilityReader struct {
	Hub magicevent.Hub
}

func (r MCPCapabilityReader) List(ctx context.Context) ([]*mcpmodel.Capability, error) {
	if r.Hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandListCapabilities, "application/workorchd/readmodel", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ListCapabilitiesCommand{})
	ownerevent.BindLane(event, mcpevents.TopicMCPCommandListCapabilities, "mcp_capability", "all", "", "", "")
	result, err := ownerevent.MCP(r.Hub.Send(event), mcpevents.TopicMCPCommandListCapabilities)
	if err != nil {
		return nil, err
	}
	return result.Capabilities, nil
}
