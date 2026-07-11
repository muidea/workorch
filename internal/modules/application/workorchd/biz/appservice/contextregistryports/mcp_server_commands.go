package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreateMCPServer(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.MCPServer) (contextregistrycore.MCPServer, error) {
	if hub == nil {
		return contextregistrycore.MCPServer{}, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[mcpevents.CreateServerCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandCreateServer, mcpevents.CommandDestination, "mcp_server", cmd.ID, mcpevents.CreateServerCommand(mcpServerCommandFromView(cmd)))
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
	result, err := sendMagicCommand[mcpevents.UpdateServerCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandUpdateServer, mcpevents.CommandDestination, "mcp_server", cmd.ID, mcpevents.UpdateServerCommand(mcpServerCommandFromView(cmd)))
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
	result, err := sendMagicCommand[mcpevents.ListServersCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandListServers, mcpevents.CommandDestination, "mcp_server", "all", mcpevents.ListServersCommand{})
	if err != nil {
		return nil, err
	}
	return mcpServersFromCommandResult(result)
}

func ReadMCPServer(ctx context.Context, hub magicevent.Hub, serverID string) (*mcpmodel.Server, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[mcpevents.GetServerCommand, mcpevents.MCPCommandResult](ctx, hub, mcpevents.TopicMCPCommandGetServer, mcpevents.CommandDestination, "mcp_server", serverID, mcpevents.GetServerCommand{ServerID: common.MCPServerID(serverID)})
	if err != nil {
		return nil, err
	}
	if result.Server == nil {
		return nil, common.ErrInvalidArgument
	}
	return result.Server, nil
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
	return contextregistrycore.MapMCPServer(result.Server), nil
}

func mcpServersFromCommandResult(result mcpevents.MCPCommandResult) ([]contextregistrycore.MCPServer, error) {
	ret := make([]contextregistrycore.MCPServer, 0, len(result.Servers))
	for _, item := range result.Servers {
		ret = append(ret, contextregistrycore.MapMCPServer(item))
	}
	return ret, nil
}
