package activationcore

func CloneContextHints(input map[string]any) map[string]any {
	return cloneContextMapDeep(input)
}

func MergeTemplateWorkspaceToolContextHints(templateHints map[string]any, workspaceHints map[string]any) map[string]any {
	if len(templateHints) == 0 {
		return cloneContextMapDeep(workspaceHints)
	}
	if len(workspaceHints) == 0 {
		return cloneContextMapDeep(templateHints)
	}
	return mergeContextMapsWorkspaceOverride(templateHints, workspaceHints)
}

func mergeContextMapsWorkspaceOverride(base map[string]any, overlay map[string]any) map[string]any {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	ret := cloneContextMapDeep(base)
	if ret == nil {
		ret = map[string]any{}
	}
	for key, value := range overlay {
		baseMap, baseOK := ret[key].(map[string]any)
		overlayMap, overlayOK := value.(map[string]any)
		if baseOK && overlayOK {
			ret[key] = mergeContextMapsWorkspaceOverride(baseMap, overlayMap)
			continue
		}
		ret[key] = cloneContextValue(value)
	}
	return ret
}
