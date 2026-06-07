package readmodelprojection

func projectMCPToolCallResult(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	ret := map[string]any{}
	if structured := projectMCPStructuredContent(raw["structuredContent"]); len(structured) > 0 {
		ret["structuredContent"] = structured
	}
	if isError, ok := raw["isError"].(bool); ok {
		ret["isError"] = isError
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
