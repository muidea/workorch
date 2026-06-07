package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func UsedRuntimeCapabilities(snapshot readmodelcore.RunSnapshot, catalog map[string]readmodelcore.RuntimeCapability) []readmodelcore.RuntimeCapability {
	if len(snapshot.ToolCalls) == 0 {
		return nil
	}
	selected := runtimeCapabilityIndex(snapshot.SelectedCapabilities)
	used := map[string]readmodelcore.RuntimeCapability{}
	order := make([]string, 0)
	for _, toolCall := range snapshot.ToolCalls {
		identity := RuntimeCapabilityIdentity(toolCall)
		if identity.ID == "" {
			continue
		}
		if _, exists := used[identity.ID]; !exists {
			order = append(order, identity.ID)
		}
		used[identity.ID] = mergeUsedRuntimeCapability(used[identity.ID], identity, catalog, selected, toolCall.ID, toolCall.StepID)
	}
	ret := make([]readmodelcore.RuntimeCapability, 0, len(order))
	for _, id := range order {
		ret = append(ret, used[id])
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
