package workspacecore

func TemplateRequestsGitInit(contextHints map[string]any) bool {
	if len(contextHints) == 0 {
		return false
	}
	versionControl, ok := contextHints["versionControl"].(map[string]any)
	if !ok {
		return false
	}
	gitConfig, ok := versionControl["git"].(map[string]any)
	if !ok {
		return false
	}
	value, ok := gitConfig["initializeOnCreate"].(bool)
	return ok && value
}
