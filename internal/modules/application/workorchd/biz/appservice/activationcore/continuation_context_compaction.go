package activationcore

func CompactContextValue(value any, depth int) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if len(typed) > 4096 {
			return typed[:4096] + "..."
		}
		return typed
	case map[string]any:
		if depth >= 3 {
			return map[string]any{"entries": len(typed)}
		}
		ret := map[string]any{}
		for key, item := range typed {
			if ContextKeySkipped(key) {
				continue
			}
			ret[key] = CompactContextValue(item, depth+1)
			if len(ret) >= 32 {
				break
			}
		}
		return ret
	case []map[string]any:
		if depth >= 3 {
			return map[string]any{"items": len(typed)}
		}
		limit := min(len(typed), 20)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, CompactContextValue(typed[idx], depth+1))
		}
		return ret
	case []any:
		if depth >= 3 {
			return map[string]any{"items": len(typed)}
		}
		limit := min(len(typed), 20)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, CompactContextValue(typed[idx], depth+1))
		}
		return ret
	case []string:
		limit := min(len(typed), 20)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, CompactContextValue(typed[idx], depth+1))
		}
		return ret
	default:
		return typed
	}
}
