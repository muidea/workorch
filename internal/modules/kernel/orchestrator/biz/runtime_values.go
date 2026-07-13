package orchestrator

import (
	"encoding/json"
	"strings"
	"time"
)

func mapFromValue(value any) map[string]any {
	mapped, _ := value.(map[string]any)
	return mapped
}

func mapSliceFromValue(value any) []map[string]any {
	if items, ok := value.([]map[string]any); ok {
		return items
	}
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	items := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if mapped := mapFromValue(item); mapped != nil {
			items = append(items, mapped)
		}
	}
	return items
}

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
