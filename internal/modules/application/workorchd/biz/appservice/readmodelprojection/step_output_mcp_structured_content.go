package readmodelprojection

func projectMCPStructuredContent(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"arguments": {},
		"name":      {},
		"ok":        {},
		"tool":      {},
		"transport": {},
	}
	ret := map[string]any{}
	for key, item := range raw {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if projected, ok := projectReadModelScalarMapValue(item); ok {
			ret[key] = projected
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
