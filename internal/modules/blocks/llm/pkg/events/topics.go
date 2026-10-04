package events

const (
	TopicLLMCommandCompleteProviderDocument    = "/llm/command/complete-provider-document"
	TopicLLMCommandCompleteProvider            = "/llm/command/complete-provider"
	TopicLLMCommandGenerateEmbedding           = "/llm/command/generate-embedding"
	TopicLLMCommandGetActiveConfiguration      = "/llm/command/get-active-configuration"
	TopicLLMCommandGetProviderModelCatalog     = "/llm/command/get-provider-model-catalog"
	TopicLLMCommandRefreshProviderModelCatalog = "/llm/command/refresh-provider-model-catalog"
	TopicLLMCommandValidateInferenceSelection  = "/llm/command/validate-inference-selection"
	TopicAcquireProviderCeiling                = "/llm/runtime/acquire-provider-ceiling"
	TopicAcquireRuntimeCapabilities            = "/llm/runtime-prerequisite/acquire-capabilities"
	TopicPreflightRuntimeCandidate             = "/llm/runtime-candidate/preflight"
	TopicRequestRuntimeCandidate               = "/llm/runtime-candidate/request"

	// TopicLLMUsageRecorded is the durable request audit notification for one
	// provider completion/embedding/candidate call. Its namespace is owned by
	// the LLM Block and is also the read-model topic filter.
	TopicLLMUsageRecorded = "/llm/usage/recorded"

	// NotificationDestination is the EventHub destination pattern for LLM data events.
	NotificationDestination = "/llm/consumers/#"
)
