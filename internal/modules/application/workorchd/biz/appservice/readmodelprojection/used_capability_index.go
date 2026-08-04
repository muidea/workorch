package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func runtimeCapabilityIndex(items []projectioncontract.RuntimeCapability) map[string]projectioncontract.RuntimeCapability {
	ret := map[string]projectioncontract.RuntimeCapability{}
	for _, item := range items {
		ret[item.ID] = item
	}
	return ret
}

func mergeUsedRuntimeCapability(current projectioncontract.RuntimeCapability, identity projectioncontract.RuntimeCapability, catalog map[string]projectioncontract.RuntimeCapability, selected map[string]projectioncontract.RuntimeCapability, toolCallID string, stepID string) projectioncontract.RuntimeCapability {
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
