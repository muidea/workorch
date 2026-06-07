package readmodelprojection

var stepOutputAllowedKeys = map[string]struct{}{
	"category":                     {},
	"completionState":              {},
	"error":                        {},
	"operation":                    {},
	"reason":                       {},
	"stage":                        {},
	"state":                        {},
	"status":                       {},
	"summary":                      {},
	"target":                       {},
	"targetRef":                    {},
	"workspaceExecutionSummary":    {},
	"workspaceExecutionSummaryRef": {},
}

func isStepOutputAllowedKey(key string) bool {
	if _, ok := stepOutputAllowedKeys[key]; ok {
		return true
	}
	if _, ok := stepOutputGovernanceAllowedKeys[key]; ok {
		return true
	}
	if _, ok := stepOutputRuntimeAllowedKeys[key]; ok {
		return true
	}
	_, ok := stepOutputLLMAllowedKeys[key]
	return ok
}
