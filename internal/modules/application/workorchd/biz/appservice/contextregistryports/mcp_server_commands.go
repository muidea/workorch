package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateMCPServer(ctx context.Context, hub magicevent.Hub, cmd workorchmanagementcontract.MCPServerMutation) (workorchmanagementcontract.MCPServer, error) {
	if hub == nil {
		return workorchmanagementcontract.MCPServer{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandCreateServer, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.CreateServerCommand(mcpServerCommandFromMutation(cmd)))
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandCreateServer, "mcp_server", cmd.ID, "", "", "")
	result, err := ownerevent.MCP[mcpevents.CreateServerResult](hub.Send(event), mcpevents.TopicMCPCommandCreateServer)
	if err != nil {
		return workorchmanagementcontract.MCPServer{}, err
	}
	return mcpServerFromEventDTO(result.Server)
}

func UpdateMCPServer(ctx context.Context, hub magicevent.Hub, cmd workorchmanagementcontract.MCPServerMutation) (workorchmanagementcontract.MCPServer, error) {
	if cmd.EnvironmentRefs == nil {
		existing, err := ReadMCPServer(ctx, hub, cmd.ID)
		if err != nil {
			return workorchmanagementcontract.MCPServer{}, err
		}
		cmd.EnvironmentRefs = cloneStringMap(existing.Env)
	}
	if hub == nil {
		return workorchmanagementcontract.MCPServer{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandUpdateServer, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.UpdateServerCommand(mcpServerCommandFromMutation(cmd)))
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandUpdateServer, "mcp_server", cmd.ID, "", "", "")
	result, err := ownerevent.MCP[mcpevents.UpdateServerResult](hub.Send(event), mcpevents.TopicMCPCommandUpdateServer)
	if err != nil {
		return workorchmanagementcontract.MCPServer{}, err
	}
	return mcpServerFromEventDTO(result.Server)
}

func UpdateMCPServerByID(ctx context.Context, hub magicevent.Hub, serverID string, cmd workorchmanagementcontract.MCPServerMutation) (workorchmanagementcontract.MCPServer, error) {
	serverID = strings.TrimSpace(serverID)
	cmd.ID = strings.TrimSpace(cmd.ID)
	if serverID == "" {
		return workorchmanagementcontract.MCPServer{}, common.ErrInvalidArgument
	}
	if cmd.ID != "" && cmd.ID != serverID {
		return workorchmanagementcontract.MCPServer{}, common.ErrInvalidArgument
	}
	cmd.ID = serverID
	return UpdateMCPServer(ctx, hub, cmd)
}

func ListMCPServers(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.MCPServer, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]workorchmanagementcontract.MCPServer, 0)
	var cursor common.MCPServerID
	var revision string
	seenCursors := make(map[common.MCPServerID]struct{})
	maxPages := (mcpcommon.MaxRegistryServers+mcpcommon.MaxListLimit-1)/mcpcommon.MaxListLimit + 1
	for page := 0; page < maxPages; page++ {
		event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandListServers, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ListServersCommand{CursorAfterID: cursor, Limit: mcpcommon.MaxListLimit})
		ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandListServers, "mcp_server", "all", "", "", "")
		result, err := ownerevent.MCP[mcpevents.ListServersResult](hub.Send(event), mcpevents.TopicMCPCommandListServers)
		if err != nil {
			return nil, err
		}
		if revision != "" && result.RegistryRevision != revision {
			return nil, common.ErrConflict
		}
		revision = result.RegistryRevision
		items, err := mcpServersFromEventDTOs(result.Servers)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
		if len(ret) > mcpcommon.MaxRegistryServers {
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

func ReadMCPServer(ctx context.Context, hub magicevent.Hub, serverID string) (*mcpmodel.Server, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandGetServer, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.GetServerCommand{ServerID: common.MCPServerID(serverID)})
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandGetServer, "mcp_server", serverID, "", "", "")
	result, err := ownerevent.MCP[mcpevents.GetServerResult](hub.Send(event), mcpevents.TopicMCPCommandGetServer)
	if err != nil {
		return nil, err
	}
	if result.Server == nil {
		return nil, common.ErrInvalidArgument
	}
	return result.Server.Model()
}

func ReadMCPServerView(ctx context.Context, hub magicevent.Hub, serverID string) (workorchmanagementcontract.MCPServer, error) {
	item, err := ReadMCPServer(ctx, hub, serverID)
	if err != nil {
		return workorchmanagementcontract.MCPServer{}, err
	}
	return contextregistrycore.MapMCPServer(item), nil
}

func mcpServerFromEventDTO(server *mcpevents.MCPServer) (workorchmanagementcontract.MCPServer, error) {
	if server == nil {
		return workorchmanagementcontract.MCPServer{}, common.ErrInvalidArgument
	}
	item, err := server.Model()
	if err != nil {
		return workorchmanagementcontract.MCPServer{}, err
	}
	return contextregistrycore.MapMCPServer(item), nil
}

func mcpServersFromEventDTOs(servers []*mcpevents.MCPServer) ([]workorchmanagementcontract.MCPServer, error) {
	ret := make([]workorchmanagementcontract.MCPServer, 0, len(servers))
	for _, item := range servers {
		model, err := item.Model()
		if err != nil {
			return nil, err
		}
		ret = append(ret, contextregistrycore.MapMCPServer(model))
	}
	return ret, nil
}
