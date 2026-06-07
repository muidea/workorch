package contextregistryports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListMCPServerTools(ctx context.Context, hub events.Hub, serverID string) ([]contextregistrycore.MCPTool, error) {
	server, err := ReadMCPServerView(ctx, hub, serverID)
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MCPToolsFromServer(server), nil
}

func ListMCPServerResources(ctx context.Context, hub events.Hub, serverID string) ([]contextregistrycore.MCPResource, error) {
	server, err := ReadMCPServerView(ctx, hub, serverID)
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MCPResourcesFromServer(server), nil
}

func ListMCPServerPrompts(ctx context.Context, hub events.Hub, serverID string) ([]contextregistrycore.MCPPrompt, error) {
	server, err := ReadMCPServerView(ctx, hub, serverID)
	if err != nil {
		return nil, err
	}
	return contextregistrycore.MCPPromptsFromServer(server), nil
}
