package readmodelprojection

import llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"

func CanonicalStepKind(kind string) string {
	return llmcommon.CanonicalRuntimeBlockKind(kind)
}

func CloneMap(input map[string]any) map[string]any {
	return cloneReadModelMap(input)
}

func MapValue(value any) map[string]any {
	return readModelMapValue(value)
}

func MapSliceValue(value any) []map[string]any {
	return readModelMapSliceValue(value)
}
