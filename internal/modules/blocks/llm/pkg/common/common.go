package common

import (
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = runtimemodel.RuntimeModuleLLM

const StepKindWorkspaceExecution = runtimemodel.StepKindLLMWorkspaceExecution
const StepKindWorkspacePlan = runtimemodel.StepKindLLMWorkspacePlan
const ExecutionContextLLMProviderID = "llmProviderId"
const DefaultProviderID = "default"

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
	EnvProvider                = "provider"
	EnvAPISurface              = "apiSurface"
	EnvBaseURL                 = "baseURL"
	EnvAPIKey                  = "apiKey"
	EnvModel                   = "model"
	EnvTimeout                 = "timeout"
	EnvDisabled                = "disabled"
	EnvChatCompletions         = "chatCompletions"
	EnvStreaming               = "streaming"
	EnvNonChatOperations       = "nonChatOperations"
	EnvTemperature             = "temperature"
	EnvMaxOutputTokens         = "maxOutputTokens"
	EnvReasoningEffort         = "reasoningEffort"
	EnvThinkingBudget          = "thinkingBudgetTokens"
	EnvContextProfileMode      = "contextProfileMode"
	EnvContextCapabilitySource = "contextCapabilitySource"
	EnvContextWindow           = "contextWindowTokens"
	EnvMaxInputTokens          = "maxInputTokens"
	EnvReservedOutput          = "reservedOutputTokens"
	EnvSafetyRatio             = "safetyRatio"
	EnvContextUtilization      = "contextUtilizationRatio"
	EnvBytesHardLimit          = "bytesHardLimit"
	EnvMaxRequestBytes         = "maxRequestBytes"
	EnvTokensPerByte           = "estimatedTokensPerByte"
	EnvFailoverEnabled         = "failoverEnabled"
	EnvFailoverProviders       = "failoverProviders"
	EnvCacheMode               = "cacheMode"
	EnvPromptCacheKey          = "promptCacheKey"
	EnvMessageCacheControl     = "messageCacheControl"
	EnvCacheUsageFields        = "cacheUsageFields"
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
