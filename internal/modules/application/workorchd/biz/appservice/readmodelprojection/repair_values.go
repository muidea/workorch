package readmodelprojection

import "strings"

func readModelLimitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func readModelTruncateString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func readModelJSONSize(value any) int {
	return readModelApproxValueSize(value, 0)
}

func readModelFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func readModelApproxValueSize(value any, depth int) int {
	if value == nil || depth > 8 {
		return 0
	}
	switch typed := value.(type) {
	case string:
		return len(typed)
	case bool:
		return 1
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return 8
	case []string:
		size := 0
		for _, item := range typed {
			size += len(item)
		}
		return size
	case []any:
		size := 0
		for _, item := range typed {
			size += readModelApproxValueSize(item, depth+1)
		}
		return size
	case []map[string]any:
		size := 0
		for _, item := range typed {
			size += readModelApproxValueSize(item, depth+1)
		}
		return size
	case map[string]any:
		size := 0
		for key, item := range typed {
			size += len(key) + readModelApproxValueSize(item, depth+1)
		}
		return size
	default:
		return 0
	}
}
