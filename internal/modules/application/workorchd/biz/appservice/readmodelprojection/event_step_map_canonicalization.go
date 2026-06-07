package readmodelprojection

func canonicalizeEventRecordStepMap(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	const eventRecordExecutionGraphKey = "executionGraph"
	if graph := readModelMapValue(payload[eventRecordExecutionGraphKey]); len(graph) > 0 {
		if nodes := canonicalizeEventRecordGraphNodes(graph["nodes"]); len(nodes) > 0 {
			payload[eventRecordExecutionGraphKey] = graph
			graph["nodes"] = nodes
		}
	}
	return payload
}
