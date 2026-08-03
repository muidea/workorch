package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreateMCPServer(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.MCPServer) (contextregistrycore.MCPServer, error) {
	if hub == nil {
		return contextregistrycore.MCPServer{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandCreateServer, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.CreateServerCommand(mcpServerCommandFromView(cmd)))
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandCreateServer, "mcp_server", cmd.ID, "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandCreateServer)
	if err != nil {
		return contextregistrycore.MCPServer{}, err
	}
	return mcpServerFromCommandResult(result)
}

func UpdateMCPServer(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.MCPServer) (contextregistrycore.MCPServer, error) {
	if cmd.Env == nil {
		existing, err := ReadMCPServer(ctx, hub, cmd.ID)
		if err != nil {
			return contextregistrycore.MCPServer{}, err
		}
		cmd.Env = cloneStringMap(existing.Env)
	}
	if hub == nil {
		return contextregistrycore.MCPServer{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandUpdateServer, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.UpdateServerCommand(mcpServerCommandFromView(cmd)))
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandUpdateServer, "mcp_server", cmd.ID, "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandUpdateServer)
	if err != nil {
		return contextregistrycore.MCPServer{}, err
	}
	return mcpServerFromCommandResult(result)
}

func UpdateMCPServerByID(ctx context.Context, hub magicevent.Hub, serverID string, cmd contextregistrycore.MCPServer) (contextregistrycore.MCPServer, error) {
	serverID = strings.TrimSpace(serverID)
	cmd.ID = strings.TrimSpace(cmd.ID)
	if serverID == "" {
		return contextregistrycore.MCPServer{}, common.ErrInvalidArgument
	}
	if cmd.ID != "" && cmd.ID != serverID {
		return contextregistrycore.MCPServer{}, common.ErrInvalidArgument
	}
	cmd.ID = serverID
	return UpdateMCPServer(ctx, hub, cmd)
}

func ListMCPServers(ctx context.Context, hub magicevent.Hub) ([]contextregistrycore.MCPServer, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandListServers, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ListServersCommand{})
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandListServers, "mcp_server", "all", "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandListServers)
	if err != nil {
		return nil, err
	}
	return mcpServersFromCommandResult(result)
}

func ReadMCPServer(ctx context.Context, hub magicevent.Hub, serverID string) (*mcpmodel.Server, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandGetServer, sourceModule, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.GetServerCommand{ServerID: common.MCPServerID(serverID)})
	ownerevent.BindLane(event, mcpevents.Contracts(), mcpevents.TopicMCPCommandGetServer, "mcp_server", serverID, "", "", "")
	result, err := ownerevent.MCP(hub.Send(event), mcpevents.TopicMCPCommandGetServer)
	if err != nil {
		return nil, err
	}
	if result.Server == nil {
		return nil, common.ErrInvalidArgument
	}
	return result.Server.Model()
}

func ReadMCPServerView(ctx context.Context, hub magicevent.Hub, serverID string) (contextregistrycore.MCPServer, error) {
	item, err := ReadMCPServer(ctx, hub, serverID)
	if err != nil {
		return contextregistrycore.MCPServer{}, err
	}
	return contextregistrycore.MapMCPServer(item), nil
}

func mcpServerFromCommandResult(result mcpevents.MCPCommandResult) (contextregistrycore.MCPServer, error) {
	if result.Server == nil {
		return contextregistrycore.MCPServer{}, common.ErrInvalidArgument
	}
	item, err := result.Server.Model()
	if err != nil {
		return contextregistrycore.MCPServer{}, err
	}
	return contextregistrycore.MapMCPServer(item), nil
}

func mcpServersFromCommandResult(result mcpevents.MCPCommandResult) ([]contextregistrycore.MCPServer, error) {
	ret := make([]contextregistrycore.MCPServer, 0, len(result.Servers))
	for _, item := range result.Servers {
		model, err := item.Model()
		if err != nil {
			return nil, err
		}
		ret = append(ret, contextregistrycore.MapMCPServer(model))
	}
	return ret, nil
}
