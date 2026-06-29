package clicore

func MapValue(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	if v, ok := payload[key].(map[string]any); ok {
		return v
	}
	return nil
}

func FirstMapValue(payload map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value := MapValue(payload, key); value != nil {
			return value
		}
	}
	return nil
}

func StringSliceValue(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	switch v := payload[key].(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func MapSliceValue(payload map[string]any, key string) []map[string]any {
	if payload == nil {
		return nil
	}
	switch v := payload[key].(type) {
	case []map[string]any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if item != nil {
				out = append(out, cloneMap(item))
			}
		}
		return out
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if mapped, ok := item.(map[string]any); ok {
				out = append(out, cloneMap(mapped))
			}
		}
		return out
	default:
		return nil
	}
}

func StringMapValue(payload map[string]any, key string) map[string]string {
	if payload == nil {
		return nil
	}
	switch v := payload[key].(type) {
	case map[string]string:
		out := make(map[string]string, len(v))
		for key, value := range v {
			out[key] = value
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(v))
		for key, value := range v {
			if s, ok := value.(string); ok {
				out[key] = s
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func FloatSliceValue(payload map[string]any, key string) []float64 {
	if payload == nil {
		return nil
	}
	switch v := payload[key].(type) {
	case []float64:
		return append([]float64(nil), v...)
	case []any:
		out := make([]float64, 0, len(v))
		for _, item := range v {
			switch typed := item.(type) {
			case float64:
				out = append(out, typed)
			case float32:
				out = append(out, float64(typed))
			case int:
				out = append(out, float64(typed))
			case int64:
				out = append(out, float64(typed))
			}
		}
		return out
	default:
		return nil
	}
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = CloneResultValue(value, 0)
	}
	return out
}
