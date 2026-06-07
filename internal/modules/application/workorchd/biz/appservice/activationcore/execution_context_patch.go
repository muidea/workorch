package activationcore

import "strings"

func MergeExecutionContextPatch(contextData map[string]any, patch map[string]any) {
	if len(contextData) == 0 || len(patch) == 0 {
		return
	}
	reserved := map[string]struct{}{
		"activation": {}, "workspaceId": {}, "goalId": {}, "doDId": {},
		"goal": {}, "goals": {}, "goalDefinitionPolicy": {},
		"dod": {}, "dods": {}, "doDDefinitionPolicy": {},
		"definitionChangeImpact":             {},
		"definitionChangeContinuationPolicy": {},
		"rejectedResultContinuation":         {},
		"replanContinuation":                 {},
	}
	for key, value := range patch {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := reserved[key]; ok {
			continue
		}
		contextData[key] = value
	}
}
