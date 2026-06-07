package workspacecore

func CloneContextMapSlice(input []map[string]any) []map[string]any {
	return cloneContextMapSlice(input)
}

func cloneContextMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = cloneContextValue(value)
	}
	return ret
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneContextMapSlice(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneContextMap(item))
	}
	return ret
}

func cloneContextValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		ret := make(map[string]any, len(typed))
		for key, item := range typed {
			ret[key] = cloneContextValue(item)
		}
		return ret
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			cloned, _ := cloneContextValue(item).(map[string]any)
			ret = append(ret, cloned)
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneContextValue(item))
		}
		return ret
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
