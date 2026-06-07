package clicore

import "encoding/json"

func DecodeOrZero[T any](payload map[string]any) *T {
	data, _ := json.Marshal(payload)
	var out T
	_ = json.Unmarshal(data, &out)
	return &out
}

func StringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func FirstStringValue(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := StringValue(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func BoolValue(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	if v, ok := payload[key].(bool); ok {
		return v
	}
	return false
}

func IntValue(payload map[string]any, key string) int {
	if payload == nil {
		return 0
	}
	switch v := payload[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
