package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func runtimeCapabilityIndex(items []readmodelcore.RuntimeCapability) map[string]readmodelcore.RuntimeCapability {
	ret := map[string]readmodelcore.RuntimeCapability{}
	for _, item := range items {
		ret[item.ID] = item
	}
	return ret
}

func mergeUsedRuntimeCapability(current readmodelcore.RuntimeCapability, identity readmodelcore.RuntimeCapability, catalog map[string]readmodelcore.RuntimeCapability, selected map[string]readmodelcore.RuntimeCapability, toolCallID string, stepID string) readmodelcore.RuntimeCapability {
	item := current
	if item.ID == "" {
		if loaded, exists := catalog[identity.ID]; exists {
			item = loaded
		} else if selectedItem, exists := selected[identity.ID]; exists {
			item = selectedItem
		} else {
			item = identity
		}
	}
	item.UseCount++
	item.EvidenceRefs = compactUniqueStrings(append(item.EvidenceRefs, toolCallID, stepID))
	return item
}
