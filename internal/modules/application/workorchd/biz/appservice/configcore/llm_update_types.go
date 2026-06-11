package configcore

type UpdateLLMConfigurationCommand struct {
	DefaultProviderID          *string                            `json:"defaultProviderId,omitempty"`
	DefaultEmbeddingProviderID *string                            `json:"defaultEmbeddingProviderId,omitempty"`
	ModelCapabilities          *[]LLMModelCapabilityConfiguration `json:"modelCapabilities,omitempty"`
	Providers                  *[]UpdateLLMProviderConfiguration  `json:"providers,omitempty"`
}

type UpdateLLMProviderConfiguration struct {
	ID                  string                             `json:"id"`
	Provider            *string                            `json:"provider,omitempty"`
	APISurface          *string                            `json:"apiSurface,omitempty"`
	BaseURL             *string                            `json:"baseUrl,omitempty"`
	APIKey              *string                            `json:"apiKey,omitempty"`
	Model               *string                            `json:"model,omitempty"`
	Timeout             *string                            `json:"timeout,omitempty"`
	Disabled            *bool                              `json:"disabled,omitempty"`
	ChatCompletions     *bool                              `json:"chatCompletions,omitempty"`
	Streaming           *bool                              `json:"streaming,omitempty"`
	NonChatOperations   []string                           `json:"nonChatOperations,omitempty"`
	FailoverEnabled     *bool                              `json:"failoverEnabled,omitempty"`
	FailoverProviders   []string                           `json:"failoverProviders,omitempty"`
	CacheMode           *string                            `json:"cacheMode,omitempty"`
	PromptCacheKey      *bool                              `json:"promptCacheKey,omitempty"`
	MessageCacheControl *bool                              `json:"messageCacheControl,omitempty"`
	CacheUsageFields    []string                           `json:"cacheUsageFields,omitempty"`
	InferenceProfile    *UpdateLLMProviderInferenceProfile `json:"inferenceProfile,omitempty"`
	ContextProfile      *UpdateLLMProviderContextProfile   `json:"contextProfile,omitempty"`
}

type UpdateLLMProviderInferenceProfile struct {
	Temperature          *float64 `json:"temperature,omitempty"`
	MaxOutputTokens      *int     `json:"maxOutputTokens,omitempty"`
	ReasoningEffort      *string  `json:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens *int     `json:"thinkingBudgetTokens,omitempty"`
}

type UpdateLLMProviderContextProfile struct {
	ProfileMode             *string  `json:"profileMode,omitempty"`
	ContextWindowTokens     *int     `json:"contextWindowTokens,omitempty"`
	MaxInputTokens          *int     `json:"maxInputTokens,omitempty"`
	ReservedOutputTokens    *int     `json:"reservedOutputTokens,omitempty"`
	SafetyRatio             *float64 `json:"safetyRatio,omitempty"`
	ContextUtilizationRatio *float64 `json:"contextUtilizationRatio,omitempty"`
	BytesHardLimit          *int     `json:"bytesHardLimit,omitempty"`
	MaxRequestBytes         *int     `json:"maxRequestBytes,omitempty"`
	EstimatedTokensPerByte  *float64 `json:"estimatedTokensPerByte,omitempty"`
}
