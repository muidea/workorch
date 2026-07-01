package orchestrator

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func canonicalLLMWorkspaceStepKind(kind string) string {
	return runtimemodel.CanonicalRuntimeBlockKind(kind)
}

func isLLMWorkspaceStepKind(kind string) bool {
	return canonicalLLMWorkspaceStepKind(kind) == llmWorkspaceExecutionStepKind
}

func canonicalRuntimeGraphNodeKind(kind string) string {
	return runtimemodel.CanonicalRuntimeBlockKind(kind)
}
