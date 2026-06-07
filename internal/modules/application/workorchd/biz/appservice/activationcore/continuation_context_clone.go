package activationcore

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"

func cloneContextMapDeep(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = cloneContextValue(value)
	}
	return ret
}

func CloneContextMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneContextMapDeep(item))
	}
	return ret
}

func CloneContextValue(value any) any {
	return cloneContextValue(value)
}

func cloneContextValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneContextMapDeep(typed)
	case []map[string]any:
		return CloneContextMapSlice(typed)
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneContextValue(item))
		}
		return ret
	case []string:
		return contextregistrycore.CloneStrings(typed)
	default:
		return value
	}
}
