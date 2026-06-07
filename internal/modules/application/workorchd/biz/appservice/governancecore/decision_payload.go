package governancecore

import "strings"

func DecisionPayloadMap(value any) map[string]any {
	typed, _ := value.(map[string]any)
	return typed
}

func StringFromDecisionPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func CloneDecisionPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
