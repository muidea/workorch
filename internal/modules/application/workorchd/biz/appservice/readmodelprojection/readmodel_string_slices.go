package readmodelprojection

func cloneReadModelStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, len(input))
	copy(ret, input)
	return ret
}

func readModelStringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		if len(typed) == 0 {
			return nil
		}
		ret := make([]string, len(typed))
		copy(ret, typed)
		return ret
	case []any:
		if len(typed) == 0 {
			return nil
		}
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if str := readModelStringValue(item); str != "" {
				ret = append(ret, str)
			}
		}
		if len(ret) == 0 {
			return nil
		}
		return ret
	default:
		return nil
	}
}

func readModelStringSliceMapValue(value any) map[string][]string {
	ret := map[string][]string{}
	switch typed := value.(type) {
	case map[string][]string:
		for key, values := range typed {
			if key == "" {
				continue
			}
			if refs := readModelStringSliceValue(values); len(refs) > 0 {
				ret[key] = refs
			}
		}
	case map[string]any:
		for key, values := range typed {
			if key == "" {
				continue
			}
			if refs := readModelStringSliceValue(values); len(refs) > 0 {
				ret[key] = refs
			}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
