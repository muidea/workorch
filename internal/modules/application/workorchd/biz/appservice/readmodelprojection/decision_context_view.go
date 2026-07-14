package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

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

func ProjectDecisionContextView(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	if len(input.Fields) == 0 {
		return workorcheventpayload.Payload{}
	}
	ret := make([]workorcheventpayload.Field, 0, len(decisionContextViewKeys))
	for _, key := range decisionContextViewKeys {
		value, ok := input.Field(key)
		if !ok {
			continue
		}
		if projected, ok := projectDecisionContextValue(key, value, 0); ok {
			ret = append(ret, workorcheventpayload.Field{Name: key, Value: projected})
		}
	}
	return workorcheventpayload.New(ret...)
}

func ProjectDecisionPayloadView(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	projected, ok := projectDecisionContextMap(input, 1)
	if !ok {
		return workorcheventpayload.Payload{}
	}
	return projected
}

func projectDecisionContextValue(key string, value workorcheventpayload.Value, depth int) (workorcheventpayload.Value, bool) {
	if depth > 2 {
		return workorcheventpayload.Value{}, false
	}
	switch value.Kind {
	case workorcheventpayload.ValueNull:
		return workorcheventpayload.Value{}, false
	case workorcheventpayload.ValueString:
		value.String = trimDecisionContextString(value.String)
		return value, true
	case workorcheventpayload.ValueBool, workorcheventpayload.ValueInt, workorcheventpayload.ValueUint, workorcheventpayload.ValueFloat:
		return value, true
	case workorcheventpayload.ValueArray:
		limit := decisionContextSliceLimit
		if key == "recentFailures" {
			limit = decisionContextRecentErrorLimit
		}
		ret := make([]workorcheventpayload.Value, 0, min(len(value.Array), limit))
		for idx, item := range value.Array {
			if idx >= limit {
				break
			}
			projected, ok := projectDecisionContextValue(key, item, depth+1)
			if ok {
				ret = append(ret, projected)
			}
		}
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: ret}, len(ret) > 0
	case workorcheventpayload.ValueObject:
		projected, ok := projectDecisionContextMap(workorcheventpayload.Payload{Fields: value.Object}, depth+1)
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject, Object: projected.Fields}, ok
	default:
		return workorcheventpayload.Value{}, false
	}
}

func projectDecisionContextMap(input workorcheventpayload.Payload, depth int) (workorcheventpayload.Payload, bool) {
	if len(input.Fields) == 0 || depth > 2 {
		return workorcheventpayload.Payload{}, false
	}
	limit := decisionContextNestedMapLimit
	if depth == 1 {
		limit = decisionContextMapLimit
	}
	ret := make([]workorcheventpayload.Field, 0, min(len(input.Fields), limit))
	count := 0
	for _, key := range orderedDecisionContextMapKeys(input) {
		if count >= limit {
			break
		}
		value, _ := input.Field(key)
		projected, ok := projectDecisionContextValue(key, value, depth)
		if !ok {
			continue
		}
		ret = append(ret, workorcheventpayload.Field{Name: key, Value: projected})
		count++
	}
	return workorcheventpayload.New(ret...), len(ret) > 0
}

func orderedDecisionContextMapKeys(input workorcheventpayload.Payload) []string {
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
		"identifiers",
		"messages",
		"sources",
	}
	ret := make([]string, 0, len(input.Fields))
	seen := map[string]struct{}{}
	for _, key := range preferred {
		if _, ok := input.Field(key); ok {
			ret = append(ret, key)
			seen[key] = struct{}{}
		}
	}
	for _, field := range input.Fields {
		if _, ok := seen[field.Name]; ok {
			continue
		}
		ret = append(ret, field.Name)
	}
	return ret
}

func trimDecisionContextString(input string) string {
	if len(input) <= decisionContextStringLimit {
		return input
	}
	return input[:decisionContextStringLimit] + "..."
}
