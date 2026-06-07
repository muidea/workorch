package activationcore

func StripRuntimeLLMBinding(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	for _, key := range []string{
		"llmProviderId",
		"providerId",
		"provider",
		"model",
	} {
		delete(input, key)
	}
	for key, value := range input {
		input[key] = StripRuntimeLLMBindingValue(value)
	}
	return input
}

func StripRuntimeLLMBindingValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return StripRuntimeLLMBinding(typed)
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, StripRuntimeLLMBinding(item))
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, StripRuntimeLLMBindingValue(item))
		}
		return ret
	default:
		return value
	}
}
