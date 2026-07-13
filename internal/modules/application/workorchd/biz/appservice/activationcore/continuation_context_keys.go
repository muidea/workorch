package activationcore

import "strings"

func ContextKeySkipped(key string) bool {
	switch strings.TrimSpace(key) {
	case "failedStepInput",
		"previousStepOutput",
		"workspaceFileManifest",
		"failureTargetRepairContext",
		"patchRepairContext",
		"content",
		"diff",
		"rawContentPreview",
		"repairRawContentPreview",
		"streamEvents",
		"llmProviderId",
		"providerId",
		"provider",
		"model":
		return true
	default:
		return false
	}
}
