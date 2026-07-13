package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
)

func RuntimeCapabilityCatalogFromItems(items []*mcpmodel.Capability) map[string]readmodelcore.RuntimeCapability {
	if items == nil {
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
