package orchestrator

import (
	"encoding/json"
	"strings"
	"time"
)

func stringFromMap(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func floatFromValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		ret, _ := typed.Float64()
		return ret
	default:
		return 0
	}
}

func int64FromValue(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case json.Number:
		ret, _ := typed.Int64()
		return ret
	default:
		return 0
	}
}

func boolFromValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func runtimeEvidenceTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
