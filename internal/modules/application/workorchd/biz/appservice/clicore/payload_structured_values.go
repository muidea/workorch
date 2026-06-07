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
