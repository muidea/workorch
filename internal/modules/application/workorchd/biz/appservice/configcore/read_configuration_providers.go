package configcore

import (
	"sort"

	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func llmProviderConfigurations(cfg workorchconfig.Config) (string, []LLMProviderConfiguration, error) {
	defaultProviderID, providerConfigs, err := llmproviderconfig.LoadProviderConfigs(cfg.LLM.DefaultProvider, cfg.LLMProviderValueSets())
	if err != nil {
		return "", nil, err
	}
	providers := make([]LLMProviderConfiguration, 0, len(providerConfigs))
	for id, providerConfig := range providerConfigs {
		redacted := llmproviderconfig.RedactedProviderConfig(providerConfig)
		rawProvider := cfg.LLM.Providers[id]
		providers = append(providers, LLMProviderConfiguration{
			ID:                redacted.ID,
			Provider:          redacted.Provider,
			BaseURL:           redacted.BaseURL,
			Model:             redacted.Model,
			Timeout:           redacted.Timeout,
			Disabled:          redacted.Disabled,
			Ready:             redacted.Ready,
			Reason:            redacted.Reason,
			Default:           id == defaultProviderID,
			APIKeyConfigured:  rawProvider.APIKey != "",
			ChatCompletions:   redacted.Capabilities.ChatCompletions,
			Streaming:         redacted.Capabilities.Streaming,
			NonChatOperations: append([]string(nil), redacted.Capabilities.NonChatOperations...),
			FailoverEnabled:   redacted.Failover.Enabled,
			FailoverProviders: append([]string(nil), redacted.Failover.Providers...),
			InferenceProfile:  redacted.InferenceProfile,
			ContextProfile:    redacted.ContextProfile,
		})
	}
	sort.Slice(providers, func(i, j int) bool {
		if providers[i].Default != providers[j].Default {
			return providers[i].Default
		}
		return providers[i].ID < providers[j].ID
	})
	return defaultProviderID, providers, nil
}
