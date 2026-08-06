package models

import "time"

type ProviderConfig struct {
	ID                string
	Provider          string
	APISurface        string
	BaseURL           string
	APIKey            string
	Model             string
	Timeout           time.Duration
	Disabled          bool
	Capabilities      ProviderCapabilities
	CacheCapability   ProviderCacheCapability
	InferenceProfile  ProviderInferenceProfile
	ContextProfile    ProviderContextProfile
	FailoverEnabled   bool
	FailoverProviders []string
	Unavailable       bool
	UnavailableReason string
}

type ProviderInferenceProfile struct {
	Temperature          *float64 `json:"temperature,omitempty"`
	MaxOutputTokens      int      `json:"maxOutputTokens,omitempty"`
	ReasoningEffort      string   `json:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens int      `json:"thinkingBudgetTokens,omitempty"`
}

type ProviderCapabilities struct {
	ChatCompletions    bool     `json:"chatCompletions"`
	Responses          bool     `json:"responses,omitempty"`
	Streaming          bool     `json:"streaming,omitempty"`
	JSONSchemaResponse bool     `json:"jsonSchemaResponse,omitempty"`
	NonChatOperations  []string `json:"nonChatOperations,omitempty"`
}

type ProviderCacheCapability struct {
	Surface             string   `json:"surface,omitempty"`
	Mode                string   `json:"mode,omitempty"`
	PromptCacheKey      bool     `json:"promptCacheKey,omitempty"`
	MessageCacheControl bool     `json:"messageCacheControl,omitempty"`
	UsageFields         []string `json:"usageFields,omitempty"`
}

type ProviderStreamEvent struct {
	Type    string         `json:"type"`
	Delta   string         `json:"delta,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type ProviderEmbeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model,omitempty"`
}

type ProviderEmbeddingResult struct {
	ProviderID string    `json:"providerId"`
	Model      string    `json:"model"`
	Input      string    `json:"input,omitempty"`
	Vector     []float64 `json:"vector"`
}

type RedactedProviderConfig struct {
	ID               string                   `json:"id,omitempty"`
	Provider         string                   `json:"provider,omitempty"`
	APISurface       string                   `json:"apiSurface,omitempty"`
	BaseURL          string                   `json:"baseUrl,omitempty"`
	Model            string                   `json:"model,omitempty"`
	Timeout          string                   `json:"timeout,omitempty"`
	Disabled         bool                     `json:"disabled"`
	Ready            bool                     `json:"ready"`
	Reason           string                   `json:"reason,omitempty"`
	Capabilities     ProviderCapabilities     `json:"capabilities"`
	CacheCapability  ProviderCacheCapability  `json:"cacheCapability,omitempty"`
	InferenceProfile ProviderInferenceProfile `json:"inferenceProfile,omitempty"`
	ContextProfile   ProviderContextProfile   `json:"contextProfile,omitempty"`
	Failover         ProviderFailoverConfig   `json:"failover,omitempty"`
}

type ProviderFailoverConfig struct {
	Enabled   bool     `json:"enabled"`
	Providers []string `json:"providers,omitempty"`
}
