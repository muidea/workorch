package activationcore

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
)

func MapSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return contextregistrycore.CloneStrings(typed)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func mapValue(value any) map[string]any {
	typed, _ := value.(map[string]any)
	return typed
}

func definitionIDsFromContexts(items []map[string]any, key string) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(stringFromMap(item, key)); id != "" {
			ret = append(ret, id)
		}
	}
	return ret
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}

func stringFromPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}
