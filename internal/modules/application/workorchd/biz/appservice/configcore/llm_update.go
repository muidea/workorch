package configcore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func UpdateLLMConfiguration(cmd UpdateLLMConfigurationCommand) (ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	next := cfg.LLM
	if cmd.AIProxy != nil {
		aiProxy := *cmd.AIProxy
		if cmd.ClearAIProxyAPIKey && strings.TrimSpace(aiProxy.APIKey) != "" {
			return ServiceConfiguration{}, fmt.Errorf("%w: clearAIProxyApiKey cannot be combined with a non-empty aiProxy.apiKey", common.ErrInvalidArgument)
		}
		// Explicit clear empties the key; blank without clear preserves the existing secret.
		if cmd.ClearAIProxyAPIKey {
			aiProxy.APIKey = ""
		} else if strings.TrimSpace(aiProxy.APIKey) == "" {
			aiProxy.APIKey = next.AIProxy.APIKey
		}
		if aiProxy.ModelOverrides == nil {
			aiProxy.ModelOverrides = cloneAIProxyModelOverrides(next.AIProxy.ModelOverrides)
		}
		next.AIProxy = aiProxy
	} else if cmd.ClearAIProxyAPIKey {
		// Allow clearing the ai-proxy key without resubmitting the full aiProxy block.
		next.AIProxy.APIKey = ""
	}
	if cmd.DefaultChat != nil {
		next.DefaultChat = cmd.DefaultChat.Normalized()
	}
	if cmd.ClearDefaultEmbedding {
		next.DefaultEmbedding = nil
	} else if cmd.DefaultEmbedding != nil {
		normalized := cmd.DefaultEmbedding.Normalized()
		next.DefaultEmbedding = &normalized
	}
	if cmd.Budget != nil {
		next.Budget = *cmd.Budget
	}
	clearKeys := map[string]struct{}{}
	for _, id := range cmd.ClearCustomProviderAPIKeys {
		id = strings.TrimSpace(id)
		if id != "" {
			clearKeys[id] = struct{}{}
		}
	}
	if cmd.CustomProviders != nil {
		merged := cloneCustomProviders(*cmd.CustomProviders)
		// Fail-fast when clear and non-empty new secret appear for the same provider.
		for id, provider := range merged {
			if _, clear := clearKeys[id]; clear && strings.TrimSpace(provider.APIKey) != "" {
				return ServiceConfiguration{}, fmt.Errorf("%w: clearCustomProviderApiKeys cannot be combined with a non-empty customProviders[%s].apiKey", common.ErrInvalidArgument, id)
			}
		}
		// Preserve existing custom provider API keys when update leaves them blank,
		// unless the provider is explicitly listed for key clear.
		for id, provider := range merged {
			if _, clear := clearKeys[id]; clear {
				provider.APIKey = ""
				merged[id] = provider
				continue
			}
			if strings.TrimSpace(provider.APIKey) != "" {
				continue
			}
			if existing, ok := next.CustomProviders[id]; ok {
				provider.APIKey = existing.APIKey
				merged[id] = provider
			}
		}
		next.CustomProviders = merged
	} else if len(clearKeys) > 0 {
		// Allow clearing keys without resubmitting full customProviders map.
		merged := cloneCustomProviders(next.CustomProviders)
		for id := range clearKeys {
			if provider, ok := merged[id]; ok {
				provider.APIKey = ""
				merged[id] = provider
			}
		}
		next.CustomProviders = merged
	}
	normalized, err := workorchconfig.NormalizeLLMConfig(next)
	if err != nil {
		return ServiceConfiguration{}, err
	}
	cfg.LLM = normalized
	if err := workorchconfig.Save(cfg); err != nil {
		return ServiceConfiguration{}, err
	}
	if err := workorchconfig.Reload(); err != nil {
		return ServiceConfiguration{}, err
	}
	return ReadServiceConfiguration()
}

// Ensure package common import is used if validation paths need it later.
var _ = common.ErrInvalidArgument
