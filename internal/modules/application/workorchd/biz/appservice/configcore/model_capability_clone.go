package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

func cloneLLMModelCapabilityConfigs(source map[string]workorchconfig.LLMModelCapabilityConfig) map[string]workorchconfig.LLMModelCapabilityConfig {
	if len(source) == 0 {
		return map[string]workorchconfig.LLMModelCapabilityConfig{}
	}
	ret := make(map[string]workorchconfig.LLMModelCapabilityConfig, len(source))
	for id, capability := range source {
		ret[id] = workorchconfig.LLMModelCapabilityConfig{
			Provider:            capability.Provider,
			ModelPattern:        capability.ModelPattern,
			ContextWindowTokens: capability.ContextWindowTokens,
		}
	}
	return ret
}
