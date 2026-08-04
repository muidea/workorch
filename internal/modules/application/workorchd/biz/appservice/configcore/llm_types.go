package configcore

import (
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type LLMConfiguration struct {
	AIProxy          AIProxyConfigurationView                          `json:"aiProxy"`
	DefaultChat      runtimecontract.ProviderModelTarget               `json:"defaultChat"`
	DefaultEmbedding *runtimecontract.ProviderModelTarget              `json:"defaultEmbedding,omitempty"`
	Budget           workorchconfig.LLMContextBudgetPolicyConfig       `json:"budget"`
	CustomProviders  []CustomProviderConfigurationView                 `json:"customProviders,omitempty"`
	Catalog          workorchmanagementcontract.ProviderCatalogSummary `json:"catalog"`
}

type AIProxyConfigurationView struct {
	BaseURL                 string `json:"baseUrl"`
	APIKeyConfigured        bool   `json:"apiKeyConfigured"`
	Timeout                 string `json:"timeout,omitempty"`
	CatalogRequestTimeout   string `json:"catalogRequestTimeout,omitempty"`
	CatalogTTL              string `json:"catalogTTL,omitempty"`
	CatalogMaxStaleAge      string `json:"catalogMaxStaleAge,omitempty"`
	MaxCatalogResponseBytes int    `json:"maxCatalogResponseBytes,omitempty"`
	MaxCatalogModels        int    `json:"maxCatalogModels,omitempty"`
}

type CustomProviderConfigurationView struct {
	ID               string                    `json:"id"`
	Provider         string                    `json:"provider"`
	APISurface       string                    `json:"apiSurface,omitempty"`
	BaseURL          string                    `json:"baseUrl,omitempty"`
	Timeout          string                    `json:"timeout,omitempty"`
	APIKeyConfigured bool                      `json:"apiKeyConfigured"`
	Models           []CustomProviderModelView `json:"models"`
}

type CustomProviderModelView struct {
	ID                   string   `json:"id"`
	ContextWindowTokens  int      `json:"contextWindowTokens"`
	MaxOutputTokens      int      `json:"maxOutputTokens"`
	Operations           []string `json:"operations"`
	Temperature          *float64 `json:"temperature,omitempty"`
	ReasoningEffort      string   `json:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens int      `json:"thinkingBudgetTokens,omitempty"`
	CacheMode            string   `json:"cacheMode,omitempty"`
	PromptCacheKey       *bool    `json:"promptCacheKey,omitempty"`
	MessageCacheControl  *bool    `json:"messageCacheControl,omitempty"`
	CacheUsageFields     []string `json:"cacheUsageFields,omitempty"`
}
