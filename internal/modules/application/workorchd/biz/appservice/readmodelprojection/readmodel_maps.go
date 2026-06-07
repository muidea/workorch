package readmodelprojection

func cloneReadModelMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneReadModelIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]int, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func readModelMapValue(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok || len(typed) == 0 {
		return nil
	}
	return cloneReadModelMap(typed)
}
