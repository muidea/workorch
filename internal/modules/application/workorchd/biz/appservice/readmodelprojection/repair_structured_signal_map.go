package readmodelprojection

func readModelStructuredFailureSignalMap(sources []string, targetRefs []string, messages []string) map[string]any {
	ret := map[string]any{}
	if sources = compactUniqueStrings(sources); len(sources) > 0 {
		ret["sources"] = readModelLimitStrings(sources, 8)
	}
	if targetRefs = compactUniqueStrings(targetRefs); len(targetRefs) > 0 {
		ret["targetRefs"] = readModelLimitStrings(targetRefs, 16)
	}
	if messages = compactUniqueStrings(messages); len(messages) > 0 {
		ret["messages"] = readModelLimitStrings(messages, 10)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
