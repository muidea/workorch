package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

type UpdateLLMConfigurationCommand struct {
	AIProxy               *workorchconfig.AIProxyProviderConfig              `json:"aiProxy,omitempty"`
	ClearAIProxyAPIKey    bool                                               `json:"clearAIProxyApiKey,omitempty"`
	DefaultChat           *workorchconfig.ProviderModelTargetConfig          `json:"defaultChat,omitempty"`
	DefaultEmbedding      *workorchconfig.ProviderModelTargetConfig          `json:"defaultEmbedding,omitempty"`
	ClearDefaultEmbedding bool                                               `json:"clearDefaultEmbedding,omitempty"`
	Budget                *workorchconfig.LLMContextBudgetPolicyConfig       `json:"budget,omitempty"`
	CustomProviders       *map[string]workorchconfig.CustomLLMProviderConfig `json:"customProviders,omitempty"`
	// ClearCustomProviderAPIKeys lists custom provider IDs whose API keys should be cleared.
	ClearCustomProviderAPIKeys []string `json:"clearCustomProviderApiKeys,omitempty"`
}
