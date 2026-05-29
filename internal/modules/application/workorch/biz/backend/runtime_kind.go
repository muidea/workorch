package backend

import llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"

func canonicalRuntimeStepKind(kind string) string {
	return llmcommon.CanonicalRuntimeBlockKind(kind)
}
