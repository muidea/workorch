package configcore

import (
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type LLMConfiguration struct {
	AIProxy          AIProxyConfigurationView             `json:"aiProxy"`
	DefaultChat      runtimecontract.ProviderModelTarget  `json:"defaultChat"`
	DefaultEmbedding *runtimecontract.ProviderModelTarget `json:"defaultEmbedding,omitempty"`
	Budget           LLMContextBudgetPolicyView           `json:"budget"`
	CustomProviders  []CustomProviderConfigurationView    `json:"customProviders,omitempty"`
	Catalog          ProviderCatalogSummary               `json:"catalog"`
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

type LLMContextBudgetPolicyView struct {
	ContextUtilizationRatio float64 `json:"contextUtilizationRatio"`
	EstimatedTokensPerByte  float64 `json:"estimatedTokensPerByte"`
	DefaultMaxOutputTokens  int     `json:"defaultMaxOutputTokens"`
	MaxRequestBytes         int     `json:"maxRequestBytes"`
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

type ProviderCatalogSummary struct {
	ProviderID   string                         `json:"providerId"`
	State        llmmodels.ProviderCatalogState `json:"state"`
	Revision     string                         `json:"revision,omitempty"`
	FetchedAt    string                         `json:"fetchedAt,omitempty"`
	ExpiresAt    string                         `json:"expiresAt,omitempty"`
	Stale        bool                           `json:"stale"`
	Source       string                         `json:"source,omitempty"`
	ErrorCode    string                         `json:"errorCode,omitempty"`
	ErrorMessage string                         `json:"errorMessage,omitempty"`
	Models       []ProviderModelDescriptorView  `json:"models,omitempty"`
}

type ProviderModelDescriptorView struct {
	ProviderID          string   `json:"providerId"`
	ModelID             string   `json:"modelId"`
	RouteOwner          string   `json:"routeOwner,omitempty"`
	ContextWindowTokens int      `json:"contextWindowTokens,omitempty"`
	MaxOutputTokens     int      `json:"maxOutputTokens,omitempty"`
	Operations          []string `json:"operations,omitempty"`
	CapabilityReady     bool     `json:"capabilityReady"`
	UnavailableReason   string   `json:"unavailableReason,omitempty"`
	CapabilityRevision  string   `json:"capabilityRevision,omitempty"`
}
