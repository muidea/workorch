package activationcore

func CompactFailedOutput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"stage",
		"status",
		"error",
		"reason",
		"failureClass",
		"failureSignature",
		"failureCategory",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = CompactContextValue(value, 1)
		}
	}
	if graph := mapValue(input["executionGraph"]); len(graph) > 0 {
		if nodes, ok := graph["nodes"].([]any); ok {
			ret["rejectedExecutionGraph"] = map[string]any{"nodeCount": len(nodes)}
		} else if nodes, ok := graph["nodes"].([]map[string]any); ok {
			ret["rejectedExecutionGraph"] = map[string]any{"nodeCount": len(nodes)}
		}
	}
	if _, ok := input["rawContentPreview"]; ok {
		ret["rawContentPreviewOmitted"] = true
	}
	if _, ok := input["repairRawContentPreview"]; ok {
		ret["repairRawContentPreviewOmitted"] = true
	}
	if _, ok := input["streamEvents"]; ok {
		ret["streamEventsOmitted"] = true
	}
	if ret["rawContentPreviewOmitted"] == true ||
		ret["repairRawContentPreviewOmitted"] == true ||
		ret["streamEventsOmitted"] == true {
		ret["omittedContentReason"] = "invalid or rejected LLM decision content is not reusable repair input; rebuild from current context, target snapshots, or fresh reads"
	}
	return ret
}
