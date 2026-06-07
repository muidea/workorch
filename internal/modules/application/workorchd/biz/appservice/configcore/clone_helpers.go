package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

func cloneLLMProviderConfigs(source map[string]workorchconfig.LLMProviderConfig) map[string]workorchconfig.LLMProviderConfig {
	if len(source) == 0 {
		return map[string]workorchconfig.LLMProviderConfig{}
	}
	ret := make(map[string]workorchconfig.LLMProviderConfig, len(source))
	for id, provider := range source {
		ret[id] = cloneLLMProviderConfig(provider)
	}
	return ret
}

func cloneLLMProviderConfig(source workorchconfig.LLMProviderConfig) workorchconfig.LLMProviderConfig {
	return workorchconfig.LLMProviderConfig{
		Provider:                source.Provider,
		BaseURL:                 source.BaseURL,
		APIKey:                  source.APIKey,
		Model:                   source.Model,
		Timeout:                 source.Timeout,
		Disabled:                cloneBoolPtr(source.Disabled),
		ChatCompletions:         cloneBoolPtr(source.ChatCompletions),
		Streaming:               cloneBoolPtr(source.Streaming),
		NonChatOperations:       append([]string(nil), source.NonChatOperations...),
		FailoverEnabled:         cloneBoolPtr(source.FailoverEnabled),
		FailoverProviders:       append([]string(nil), source.FailoverProviders...),
		Temperature:             cloneFloat64Ptr(source.Temperature),
		MaxOutputTokens:         source.MaxOutputTokens,
		ReasoningEffort:         source.ReasoningEffort,
		ThinkingBudgetTokens:    source.ThinkingBudgetTokens,
		ContextProfileMode:      source.ContextProfileMode,
		ContextWindowTokens:     source.ContextWindowTokens,
		MaxInputTokens:          source.MaxInputTokens,
		ReservedOutputTokens:    source.ReservedOutputTokens,
		SafetyRatio:             source.SafetyRatio,
		ContextUtilizationRatio: source.ContextUtilizationRatio,
		BytesHardLimit:          source.BytesHardLimit,
		MaxRequestBytes:         source.MaxRequestBytes,
		EstimatedTokensPerByte:  source.EstimatedTokensPerByte,
	}
}
