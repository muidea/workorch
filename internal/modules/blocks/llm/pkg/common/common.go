package common

import (
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = runtimemodel.RuntimeModuleLLM

const StepKindWorkspaceExecution = runtimemodel.StepKindLLMWorkspaceExecution
const StepKindWorkspacePlan = runtimemodel.StepKindLLMWorkspacePlan

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
	ProviderOpenAI     = "openai"
	ProviderCompatible = "compatible"
	ProviderAnthropic  = "anthropic"
)

const (
	ProviderSurfaceOpenAISDK               = "openai_sdk"
	ProviderSurfaceAnthropicSDK            = "anthropic_sdk"
	ProviderSurfaceOpenAICompatibleHTTP    = "openai_compatible_http"
	ProviderSurfaceAnthropicCompatibleHTTP = "anthropic_compatible_http"
)

const (
	ProviderCacheModeNone                  = "none"
	ProviderCacheModeAutoPrefix            = "auto_prefix"
	ProviderCacheModePromptCacheKey        = "prompt_cache_key"
	ProviderCacheModeAnthropicCacheControl = "anthropic_cache_control"
	ProviderCacheModeCompatibleDeclared    = "compatible_declared"
)

const (
	ProviderOperationChatCompletions = "chat_completions"
	ProviderOperationEmbeddings      = "embeddings"
)

const (
	UsageSourceEstimated      = "estimated"
	UsageSourceProviderNative = "provider_native"
)

func CanonicalRuntimeBlockKind(kind string) string {
	return runtimemodel.CanonicalRuntimeBlockKind(strings.TrimSpace(kind))
}
