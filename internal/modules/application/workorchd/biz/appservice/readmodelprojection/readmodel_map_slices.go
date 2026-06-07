package readmodelprojection

func readModelMapSliceValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		if len(typed) == 0 {
			return nil
		}
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneReadModelMap(item))
		}
		return ret
	case []any:
		if len(typed) == 0 {
			return nil
		}
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped := readModelMapValue(item); len(mapped) > 0 {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
}
