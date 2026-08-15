package readmodelprojection

import llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"

// WD5 mechanical consolidation: same import contract; source list follows.

// source: event_graph_canonicalization.go

func canonicalizeEventRecordGraphNodes(value any) []map[string]any {
	nodes := readModelMapSliceValue(value)
	if len(nodes) == 0 {
		return nil
	}
	for idx := range nodes {
		if kind, ok := nodes[idx]["kind"].(string); ok {
			nodes[idx]["kind"] = llmcommon.CanonicalRuntimeBlockKind(kind)
		}
	}
	return nodes
}

// source: event_projection_helpers.go

func canonicalStepKind(kind string) string {
	return llmcommon.CanonicalRuntimeBlockKind(kind)
}

func cloneMap(input map[string]any) map[string]any {
	return cloneReadModelMap(input)
}

func mapDynamicValue(value any) map[string]any {
	return readModelMapValue(value)
}

func mapDynamicSliceValue(value any) []map[string]any {
	return readModelMapSliceValue(value)
}
