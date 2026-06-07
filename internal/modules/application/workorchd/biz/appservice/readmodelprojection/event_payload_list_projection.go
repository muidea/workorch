package readmodelprojection

func projectEventPayloadStringListValue(values []string) []string {
	limit := min(len(values), EventPayloadListLimit)
	ret := make([]string, 0, limit)
	for _, item := range values[:limit] {
		if len(item) > EventPayloadStringLimit {
			item = item[:EventPayloadStringLimit] + "..."
		}
		ret = append(ret, item)
	}
	return ret
}

func projectEventPayloadAnyListValue(values []any) (any, bool) {
	limit := min(len(values), EventPayloadListLimit)
	ret := make([]any, 0, limit)
	for _, item := range values[:limit] {
		if projected, ok := projectEventPayloadValue(item); ok {
			ret = append(ret, projected)
		}
	}
	if len(ret) == 0 {
		return nil, false
	}
	return ret, true
}
