package contextregistryports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateMCPServer(ctx context.Context, hub events.Hub, cmd contextregistrycore.MCPServer) (contextregistrycore.MCPServer, error) {
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicMCPCommandCreateServer, "mcp_server", cmd.ID, map[string]any{
		events.PayloadKeyCommand: mcpServerCommandFromView(cmd),
	})
	if err != nil {
		return contextregistrycore.MCPServer{}, err
	}
	return contextregistrycore.MCPServerFromPayload(response.Payload, events.PayloadKeyServer)
}

func UpdateMCPServer(ctx context.Context, hub events.Hub, cmd contextregistrycore.MCPServer) (contextregistrycore.MCPServer, error) {
	if cmd.Env == nil {
		existing, err := ReadMCPServer(ctx, hub, cmd.ID)
		if err != nil {
			return contextregistrycore.MCPServer{}, err
		}
		cmd.Env = cloneStringMap(existing.Env)
	}
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicMCPCommandUpdateServer, "mcp_server", cmd.ID, map[string]any{
		events.PayloadKeyCommand: mcpServerCommandFromView(cmd),
	})
	if err != nil {
		return contextregistrycore.MCPServer{}, err
	}
	return contextregistrycore.MCPServerFromPayload(response.Payload, events.PayloadKeyServer)
}

func UpdateMCPServerByID(ctx context.Context, hub events.Hub, serverID string, cmd contextregistrycore.MCPServer) (contextregistrycore.MCPServer, error) {
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

func ListMCPServers(ctx context.Context, hub events.Hub) ([]contextregistrycore.MCPServer, error) {
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicMCPCommandListServers, "mcp_server", "all", nil)
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MCPServersFromPayload(response.Payload, events.PayloadKeyServers)
}

func ReadMCPServer(ctx context.Context, hub events.Hub, serverID string) (*mcpmodel.Server, error) {
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicMCPCommandGetServer, "mcp_server", serverID, map[string]any{
		events.PayloadKeyServerID: serverID,
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyServer].(*mcpmodel.Server)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ReadMCPServerView(ctx context.Context, hub events.Hub, serverID string) (contextregistrycore.MCPServer, error) {
	item, err := ReadMCPServer(ctx, hub, serverID)
	if err != nil {
		return contextregistrycore.MCPServer{}, err
	}
	return contextregistrycore.MapMCPServer(item), nil
}
