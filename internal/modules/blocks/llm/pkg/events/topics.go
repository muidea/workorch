package events

const (
	TopicLLMCommandCompleteProvider            = "/llm/command/complete-provider"
	TopicLLMCommandGenerateEmbedding           = "/llm/command/generate-embedding"
	TopicLLMCommandGetProviderModelCatalog     = "/llm/command/get-provider-model-catalog"
	TopicLLMCommandRefreshProviderModelCatalog = "/llm/command/refresh-provider-model-catalog"
	TopicLLMCommandResolveProviderModelTarget  = "/llm/command/resolve-provider-model-target"
	TopicAcquireProviderCeiling                = "/llm/runtime/acquire-provider-ceiling"
	TopicAcquireRuntimeCapabilities            = "/llm/runtime-prerequisite/acquire-capabilities"
	TopicPreflightRuntimeCandidate             = "/llm/runtime-candidate/preflight"
	TopicRequestRuntimeCandidate               = "/llm/runtime-candidate/request"

	// TopicLLMUsageRecorded is the durable request audit notification for one
	// provider completion/embedding/candidate call. The path is historical and
	// matches the read-model topic filter /run/llm_usage/recorded.
	TopicLLMUsageRecorded = "/run/llm_usage/recorded"

	// NotificationDestination is the EventHub destination pattern for LLM data events.
	NotificationDestination = "/llm/consumers/#"
)
