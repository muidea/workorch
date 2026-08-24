package common

import (
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = runtimemodel.RuntimeModuleLLM

const StepKindWorkspaceExecution = runtimemodel.StepKindLLMWorkspaceExecution

// RuntimeCapabilityKinds is the LLM owner's complete Runtime capability inventory.
func RuntimeCapabilityKinds() []string { return []string{StepKindWorkspaceExecution} }

const (
	RuntimeBlockKindPatchApply               = runtimemodel.RuntimeBlockKindPatchApply
	RuntimeBlockKindPatchPreview             = runtimemodel.RuntimeBlockKindPatchPreview
	RuntimeBlockKindPatchRevert              = runtimemodel.RuntimeBlockKindPatchRevert
	RuntimeBlockKindManagedValidationSession = runtimemodel.RuntimeBlockKindManagedValidationSession
	RuntimeBlockKindDecisionRequest          = runtimemodel.RuntimeBlockKindDecisionRequest
	RuntimeBlockKindApprovalRequest          = runtimemodel.RuntimeBlockKindApprovalRequest
	RuntimeBlockKindMemoryWrite              = runtimemodel.RuntimeBlockKindMemoryWrite
	RuntimeBlockKindDoDAssessment            = runtimemodel.RuntimeBlockKindDoDAssessment
)

const (
	// ProviderAIProxy is the only LLM route owner supported by WorkOrch.
	ProviderAIProxy = "ai-proxy"
	// ProviderSurfaceOpenAICompatibleHTTP is the ai-proxy OpenAI protocol
	// adapter. Runtime calls are always sent to /v1/responses.
	ProviderSurfaceOpenAICompatibleHTTP = "openai_compatible_http"
)

const ProviderOperationEmbeddings = "embeddings"

const (
	UsageSourceEstimated      = "estimated"
	UsageSourceProviderNative = "provider_native"
)

func CanonicalRuntimeBlockKind(kind string) string {
	return runtimemodel.CanonicalRuntimeBlockKind(strings.TrimSpace(kind))
}
