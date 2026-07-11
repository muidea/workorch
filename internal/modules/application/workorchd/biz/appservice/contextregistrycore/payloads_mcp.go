package contextregistrycore

import (
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func MCPServerFromPayload(payload map[string]any, key string) (MCPServer, error) {
	item, ok := payload[key].(*mcpmodel.Server)
	if !ok {
		return MCPServer{}, common.ErrInvalidArgument
	}
	return MapMCPServer(item), nil
}

func MCPServersFromPayload(payload map[string]any, key string) ([]MCPServer, error) {
	items, ok := payload[key].([]*mcpmodel.Server)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]MCPServer, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapMCPServer(item))
	}
	return ret, nil
}

func MCPCapabilityFromPayload(payload map[string]any, key string) (MCPCapability, error) {
	item, ok := payload[key].(*mcpmodel.Capability)
	if !ok {
		return MCPCapability{}, common.ErrInvalidArgument
	}
	return MapMCPCapability(item), nil
}

func MCPCapabilitiesFromPayload(payload map[string]any, key string) ([]MCPCapability, error) {
	items, ok := payload[key].([]*mcpmodel.Capability)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]MCPCapability, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapMCPCapability(item))
	}
	return ret, nil
}
