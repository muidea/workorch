package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateMCPCapability(ctx context.Context, hub magicevent.Hub, cmd workorchmanagementcontract.MCPCapabilityMutation) (workorchmanagementcontract.MCPCapability, error) {
	if hub == nil {
		return workorchmanagementcontract.MCPCapability{}, common.ErrInvalidArgument
	}
	command, err := mcpCapabilityCommandFromMutation(cmd)
	if err != nil {
		return workorchmanagementcontract.MCPCapability{}, err
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandCreateCapability, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandCreateCapability, "mcp_capability", cmd.ID, "", "", "")
	result, err := ownerevent.MCP[mcpevents.CreateCapabilityResult](hub.Send(event), mcpevents.TopicMCPCommandCreateCapability)
	if err != nil {
		return workorchmanagementcontract.MCPCapability{}, err
	}
	return mcpCapabilityFromEventModel(result.Capability)
}

func UpdateMCPCapability(ctx context.Context, hub magicevent.Hub, cmd workorchmanagementcontract.MCPCapabilityMutation) (workorchmanagementcontract.MCPCapability, error) {
	if hub == nil {
		return workorchmanagementcontract.MCPCapability{}, common.ErrInvalidArgument
	}
	command, err := mcpCapabilityCommandFromMutation(cmd)
	if err != nil {
		return workorchmanagementcontract.MCPCapability{}, err
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandUpdateCapability, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.UpdateCapabilityCommand(command))
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandUpdateCapability, "mcp_capability", cmd.ID, "", "", "")
	result, err := ownerevent.MCP[mcpevents.UpdateCapabilityResult](hub.Send(event), mcpevents.TopicMCPCommandUpdateCapability)
	if err != nil {
		return workorchmanagementcontract.MCPCapability{}, err
	}
	return mcpCapabilityFromEventModel(result.Capability)
}

func UpdateMCPCapabilityByID(ctx context.Context, hub magicevent.Hub, capabilityID string, cmd workorchmanagementcontract.MCPCapabilityMutation) (workorchmanagementcontract.MCPCapability, error) {
	capabilityID = strings.TrimSpace(capabilityID)
	cmd.ID = strings.TrimSpace(cmd.ID)
	if capabilityID == "" {
		return workorchmanagementcontract.MCPCapability{}, common.ErrInvalidArgument
	}
	if cmd.ID != "" && cmd.ID != capabilityID {
		return workorchmanagementcontract.MCPCapability{}, common.ErrInvalidArgument
	}
	cmd.ID = capabilityID
	return UpdateMCPCapability(ctx, hub, cmd)
}

func ReadMCPCapability(ctx context.Context, hub magicevent.Hub, capabilityID string) (workorchmanagementcontract.MCPCapability, error) {
	if hub == nil {
		return workorchmanagementcontract.MCPCapability{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandGetCapability, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.GetCapabilityCommand{CapabilityID: common.MCPCapabilityID(capabilityID)})
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandGetCapability, "mcp_capability", capabilityID, "", "", "")
	result, err := ownerevent.MCP[mcpevents.GetCapabilityResult](hub.Send(event), mcpevents.TopicMCPCommandGetCapability)
	if err != nil {
		return workorchmanagementcontract.MCPCapability{}, err
	}
	return mcpCapabilityFromEventModel(result.Capability)
}

func ListMCPCapabilities(ctx context.Context, hub magicevent.Hub, query contextregistrycore.MCPCapabilityQuery) ([]workorchmanagementcontract.MCPCapability, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]workorchmanagementcontract.MCPCapability, 0)
	var cursor common.MCPCapabilityID
	var revision string
	seenCursors := make(map[common.MCPCapabilityID]struct{})
	maxPages := (mcpcommon.MaxRegistryCapabilities+mcpcommon.MaxListLimit-1)/mcpcommon.MaxListLimit + 1
	for page := 0; page < maxPages; page++ {
		event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandListCapabilities, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ListCapabilitiesCommand{Query: mcpCapabilityQueryFromView(query), CursorAfterID: cursor, Limit: mcpcommon.MaxListLimit})
		ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandListCapabilities, "mcp_capability", "all", "", "", "")
		result, err := ownerevent.MCP[mcpevents.ListCapabilitiesResult](hub.Send(event), mcpevents.TopicMCPCommandListCapabilities)
		if err != nil {
			return nil, err
		}
		if revision != "" && result.RegistryRevision != revision {
			return nil, common.ErrConflict
		}
		revision = result.RegistryRevision
		ret = append(ret, mcpCapabilitiesFromEventModels(result.Capabilities)...)
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

func mcpCapabilityFromEventModel(capability *mcpmodel.Capability) (workorchmanagementcontract.MCPCapability, error) {
	if capability == nil {
		return workorchmanagementcontract.MCPCapability{}, common.ErrInvalidArgument
	}
	return contextregistrycore.MapMCPCapability(capability), nil
}

func mcpCapabilitiesFromEventModels(capabilities []*mcpmodel.Capability) []workorchmanagementcontract.MCPCapability {
	ret := make([]workorchmanagementcontract.MCPCapability, 0, len(capabilities))
	for _, item := range capabilities {
		ret = append(ret, contextregistrycore.MapMCPCapability(item))
	}
	return ret
}
