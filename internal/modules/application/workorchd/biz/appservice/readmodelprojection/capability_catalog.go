package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
)

func RuntimeCapabilityCatalogFromItems(executionContext map[string]any, toolCalls []readmodelcore.ToolCall, items []*mcpmodel.Capability) map[string]readmodelcore.RuntimeCapability {
	if items == nil || !RequiresRuntimeCapabilityCatalog(executionContext, toolCalls) {
		return nil
	}
	ret := make(map[string]readmodelcore.RuntimeCapability, len(items))
	for _, item := range items {
		capability := RuntimeCapabilityFromModel(item)
		if capability.ID == "" {
			continue
		}
		ret[capability.ID] = capability
	}
	return ret
}

func RequiresRuntimeCapabilityCatalog(executionContext map[string]any, toolCalls []readmodelcore.ToolCall) bool {
	if len(readModelStringSliceValue(executionContext["capabilityIds"])) > 0 {
		return true
	}
	for _, toolCall := range toolCalls {
		if RuntimeCapabilityIdentity(toolCall).ID != "" {
			return true
		}
	}
	return false
}
