package clicore

func CloneResultValue(value any, depth int) any {
	if depth >= 6 {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		ret := make(map[string]any, len(typed))
		for key, value := range typed {
			ret[key] = CloneResultValue(value, depth+1)
		}
		return ret
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if cloned, ok := CloneResultValue(item, depth+1).(map[string]any); ok {
				ret = append(ret, cloned)
			}
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, CloneResultValue(item, depth+1))
		}
		return ret
	default:
		return value
	}
}
