package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

func cloneCustomProviders(source map[string]workorchconfig.CustomLLMProviderConfig) map[string]workorchconfig.CustomLLMProviderConfig {
	if len(source) == 0 {
		return map[string]workorchconfig.CustomLLMProviderConfig{}
	}
	ret := make(map[string]workorchconfig.CustomLLMProviderConfig, len(source))
	for id, provider := range source {
		copied := provider
		if provider.Models != nil {
			copied.Models = make(map[string]workorchconfig.CustomLLMModelConfig, len(provider.Models))
			for modelID, model := range provider.Models {
				modelCopy := model
				modelCopy.Operations = append([]string(nil), model.Operations...)
				if model.Temperature != nil {
					value := *model.Temperature
					modelCopy.Temperature = &value
				}
				if model.Cache.PromptCacheKey != nil {
					value := *model.Cache.PromptCacheKey
					modelCopy.Cache.PromptCacheKey = &value
				}
				if model.Cache.MessageCacheControl != nil {
					value := *model.Cache.MessageCacheControl
					modelCopy.Cache.MessageCacheControl = &value
				}
				modelCopy.Cache.UsageFields = append([]string(nil), model.Cache.UsageFields...)
				copied.Models[modelID] = modelCopy
			}
		}
		ret[id] = copied
	}
	return ret
}

func cloneAIProxyModelOverrides(source map[string]workorchconfig.AIProxyModelOverrideConfig) map[string]workorchconfig.AIProxyModelOverrideConfig {
	if len(source) == 0 {
		return map[string]workorchconfig.AIProxyModelOverrideConfig{}
	}
	ret := make(map[string]workorchconfig.AIProxyModelOverrideConfig, len(source))
	for id, override := range source {
		ret[id] = override
	}
	return ret
}
