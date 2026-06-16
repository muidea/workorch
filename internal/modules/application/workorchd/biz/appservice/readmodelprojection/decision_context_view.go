package readmodelprojection

const (
	decisionContextStringLimit      = 512
	decisionContextSliceLimit       = 8
	decisionContextMapLimit         = 16
	decisionContextNestedMapLimit   = 10
	decisionContextRecentErrorLimit = 3
)

var decisionContextViewKeys = []string{
	"scope",
	"dependency",
	"recoveryPolicy",
	"resultPolicy",
	"allowedRecoveryActions",
	"preferredRecoveryActions",
	"blockerKind",
	"attempts",
	"maxAttempts",
	"contractViolationType",
	"failureClass",
	"failureSignature",
	"failureTargetScope",
	"failedStepId",
	"repairOfRootStepId",
	"failedStepKind",
	"missingDoDItems",
	"recommendedNextActions",
	"runLevelIneffectiveConvergence",
	"structuredFailureSignals",
}

func ProjectDecisionContextView(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(decisionContextViewKeys))
	for _, key := range decisionContextViewKeys {
		value, ok := input[key]
		if !ok {
			continue
		}
		if projected, ok := projectDecisionContextValue(key, value, 0); ok {
			ret[key] = projected
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func ProjectDecisionPayloadView(input map[string]any) map[string]any {
	projected, ok := projectDecisionContextMap(input, 1)
	if !ok {
		return nil
	}
	return projected
}

func projectDecisionContextValue(key string, value any, depth int) (any, bool) {
	if depth > 2 {
		return nil, false
	}
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		return trimDecisionContextString(typed), true
	case bool:
		return typed, true
	case int:
		return typed, true
	case int64:
		return typed, true
	case float64:
		return typed, true
	case []string:
		limit := decisionContextSliceLimit
		ret := make([]string, 0, min(len(typed), limit))
		for idx, item := range typed {
			if idx >= limit {
				break
			}
			ret = append(ret, trimDecisionContextString(item))
		}
		return ret, true
	case []any:
		limit := decisionContextSliceLimit
		if key == "recentFailures" {
			limit = decisionContextRecentErrorLimit
		}
		ret := make([]any, 0, min(len(typed), limit))
		for idx, item := range typed {
			if idx >= limit {
				break
			}
			projected, ok := projectDecisionContextValue(key, item, depth+1)
			if ok {
				ret = append(ret, projected)
			}
		}
		return ret, len(ret) > 0
	case map[string]any:
		return projectDecisionContextMap(typed, depth+1)
	default:
		return nil, false
	}
}

func projectDecisionContextMap(input map[string]any, depth int) (map[string]any, bool) {
	if len(input) == 0 || depth > 2 {
		return nil, false
	}
	limit := decisionContextNestedMapLimit
	if depth == 1 {
		limit = decisionContextMapLimit
	}
	ret := make(map[string]any, min(len(input), limit))
	count := 0
	for _, key := range orderedDecisionContextMapKeys(input) {
		if count >= limit {
			break
		}
		projected, ok := projectDecisionContextValue(key, input[key], depth)
		if !ok {
			continue
		}
		ret[key] = projected
		count++
	}
	return ret, len(ret) > 0
}

func orderedDecisionContextMapKeys(input map[string]any) []string {
	preferred := []string{
		"totalSteps",
		"recoverySteps",
		"contractFailures",
		"providerFailures",
		"failedLLMSteps",
		"minRecoverySteps",
		"minContractFailures",
		"recentFailures",
		"stepId",
		"stage",
		"failureClass",
		"failureSignature",
		"contractViolationType",
		"identifiers",
		"messages",
		"sources",
	}
	ret := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, key := range preferred {
		if _, ok := input[key]; ok {
			ret = append(ret, key)
			seen[key] = struct{}{}
		}
	}
	for key := range input {
		if _, ok := seen[key]; ok {
			continue
		}
		ret = append(ret, key)
	}
	return ret
}

func trimDecisionContextString(input string) string {
	if len(input) <= decisionContextStringLimit {
		return input
	}
	return input[:decisionContextStringLimit] + "..."
}
