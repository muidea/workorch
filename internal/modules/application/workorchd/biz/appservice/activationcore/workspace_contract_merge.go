package activationcore

func MergeTemplateWorkspaceContextHintsForExecutionContract(templateHints map[string]any, workspaceHints map[string]any) map[string]any {
	if len(templateHints) == 0 {
		return cloneContextMapDeep(workspaceHints)
	}
	ret := cloneContextMapDeep(templateHints)
	for key, value := range workspaceHints {
		if key == "profileContracts" {
			ret[key] = MergeTemplateWorkspaceProfileContracts(contextMapValue(templateHints[key]), contextMapValue(value))
			continue
		}
		if _, exists := ret[key]; exists {
			continue
		}
		ret[key] = cloneContextValue(value)
	}
	return ret
}

func MergeTemplateWorkspaceProfileContracts(templateProfiles map[string]any, workspaceProfiles map[string]any) map[string]any {
	if len(templateProfiles) == 0 {
		if cloned := cloneContextValue(workspaceProfiles); cloned != nil {
			if mapped, ok := cloned.(map[string]any); ok {
				return mapped
			}
		}
		return nil
	}
	ret := map[string]any{}
	for profileID, profile := range templateProfiles {
		ret[profileID] = cloneContextValue(profile)
	}
	for profileID, workspaceProfile := range workspaceProfiles {
		templateProfile, exists := ret[profileID]
		if !exists {
			ret[profileID] = cloneContextValue(workspaceProfile)
			continue
		}
		ret[profileID] = MergeTemplateWorkspaceProfileContract(contextMapValue(templateProfile), contextMapValue(workspaceProfile))
	}
	return ret
}

func MergeTemplateWorkspaceProfileContract(templateProfile map[string]any, workspaceProfile map[string]any) map[string]any {
	if len(templateProfile) == 0 {
		if cloned := cloneContextValue(workspaceProfile); cloned != nil {
			if mapped, ok := cloned.(map[string]any); ok {
				return mapped
			}
		}
		return nil
	}
	ret := map[string]any{}
	for key, value := range templateProfile {
		ret[key] = cloneContextValue(value)
	}
	for key, value := range workspaceProfile {
		if _, exists := ret[key]; exists {
			continue
		}
		ret[key] = cloneContextValue(value)
	}
	return ret
}

func contextMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}
