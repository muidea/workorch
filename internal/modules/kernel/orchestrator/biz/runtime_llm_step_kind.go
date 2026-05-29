package orchestrator

import llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"

func canonicalLLMWorkspaceStepKind(kind string) string {
	return llmcommon.CanonicalRuntimeBlockKind(kind)
}

func isLLMWorkspaceStepKind(kind string) bool {
	return canonicalLLMWorkspaceStepKind(kind) == llmWorkspaceExecutionStepKind
}

func canonicalRuntimeGraphNodeKind(kind string) string {
	return llmcommon.CanonicalRuntimeBlockKind(kind)
}
