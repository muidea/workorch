package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
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
	ret := make([]*mcpmodel.Capability, 0)
	var cursor common.MCPCapabilityID
	var revision string
	seenCursors := make(map[common.MCPCapabilityID]struct{})
	maxPages := (mcpcommon.MaxRegistryCapabilities+mcpcommon.MaxListLimit-1)/mcpcommon.MaxListLimit + 1
	for page := 0; page < maxPages; page++ {
		event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandListCapabilities, "application/workorchd/readmodel", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ListCapabilitiesCommand{CursorAfterID: cursor, Limit: mcpcommon.MaxListLimit})
		ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandListCapabilities, "mcp_capability", "all", "", "", "")
		result, err := ownerevent.MCP[mcpevents.ListCapabilitiesResult](r.Hub.Send(event), mcpevents.TopicMCPCommandListCapabilities)
		if err != nil {
			return nil, err
		}
		if revision != "" && result.RegistryRevision != revision {
			return nil, common.ErrConflict
		}
		revision = result.RegistryRevision
		ret = append(ret, mcpmodel.CloneCapabilities(result.Capabilities)...)
		if len(ret) > mcpcommon.MaxRegistryCapabilities {
			return nil, common.ErrConflict
		}
		if result.NextCursor == "" {
			return ret, nil
		}
		if _, repeated := seenCursors[result.NextCursor]; repeated {
			return nil, common.ErrConflict
		}
		seenCursors[result.NextCursor] = struct{}{}
		cursor = result.NextCursor
	}
	return nil, common.ErrConflict
}
