package readmodelprojection

import (
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func RuntimeCapabilityCatalogFromItems(items []*mcpmodel.Capability) map[string]projectioncontract.RuntimeCapability {
	if items == nil {
		return nil
	}
	ret := make(map[string]projectioncontract.RuntimeCapability, len(items))
	for _, item := range items {
		capability := RuntimeCapabilityFromModel(item)
		if capability.ID == "" {
			continue
		}
		ret[capability.ID] = capability
	}
	return ret
}
