package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func appendSelectedRuntimeCapabilitiesFromContext(ret []readmodelcore.RuntimeCapability, seen map[string]struct{}, executionContext map[string]any) []readmodelcore.RuntimeCapability {
	for _, raw := range readModelMapSliceValue(executionContext["mcpCapabilities"]) {
		item := RuntimeCapabilityFromContext(raw)
		if item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func appendSelectedRuntimeCapabilitiesFromIDs(ret []readmodelcore.RuntimeCapability, seen map[string]struct{}, executionContext map[string]any, catalog map[string]readmodelcore.RuntimeCapability) []readmodelcore.RuntimeCapability {
	for _, id := range readModelStringSliceValue(executionContext["capabilityIds"]) {
		if _, ok := seen[id]; ok {
			continue
		}
		if item, ok := catalog[id]; ok {
			ret = append(ret, item)
		} else {
			ret = append(ret, readmodelcore.RuntimeCapability{ID: id, DisplayName: id})
		}
		seen[id] = struct{}{}
	}
	return ret
}
