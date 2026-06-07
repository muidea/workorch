package readmodelprojection

import llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"

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
