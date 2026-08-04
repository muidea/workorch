package contextregistrycore

import (
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func MCPServerFromPayload(payload map[string]any, key string) (workorchmanagementcontract.MCPServer, error) {
	item, ok := payload[key].(*mcpmodel.Server)
	if !ok {
		return workorchmanagementcontract.MCPServer{}, common.ErrInvalidArgument
	}
	return MapMCPServer(item), nil
}

func MCPServersFromPayload(payload map[string]any, key string) ([]workorchmanagementcontract.MCPServer, error) {
	items, ok := payload[key].([]*mcpmodel.Server)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]workorchmanagementcontract.MCPServer, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapMCPServer(item))
	}
	return ret, nil
}

func MCPCapabilityFromPayload(payload map[string]any, key string) (workorchmanagementcontract.MCPCapability, error) {
	item, ok := payload[key].(*mcpmodel.Capability)
	if !ok {
		return workorchmanagementcontract.MCPCapability{}, common.ErrInvalidArgument
	}
	return MapMCPCapability(item), nil
}

func MCPCapabilitiesFromPayload(payload map[string]any, key string) ([]workorchmanagementcontract.MCPCapability, error) {
	items, ok := payload[key].([]*mcpmodel.Capability)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]workorchmanagementcontract.MCPCapability, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapMCPCapability(item))
	}
	return ret, nil
}
