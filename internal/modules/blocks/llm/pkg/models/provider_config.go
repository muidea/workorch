package models

import "time"

type ProviderConfig struct {
	ID                string
	Provider          string
	BaseURL           string
	APIKey            string
	Model             string
	Timeout           time.Duration
	Disabled          bool
	Capabilities      ProviderCapabilities
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
	ChatCompletions   bool     `json:"chatCompletions"`
	Streaming         bool     `json:"streaming,omitempty"`
	NonChatOperations []string `json:"nonChatOperations,omitempty"`
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
	BaseURL          string                   `json:"baseUrl,omitempty"`
	Model            string                   `json:"model,omitempty"`
	Timeout          string                   `json:"timeout,omitempty"`
	Disabled         bool                     `json:"disabled"`
	Ready            bool                     `json:"ready"`
	Reason           string                   `json:"reason,omitempty"`
	Capabilities     ProviderCapabilities     `json:"capabilities"`
	InferenceProfile ProviderInferenceProfile `json:"inferenceProfile,omitempty"`
	ContextProfile   ProviderContextProfile   `json:"contextProfile,omitempty"`
	Failover         ProviderFailoverConfig   `json:"failover,omitempty"`
}

type ProviderFailoverConfig struct {
	Enabled   bool     `json:"enabled"`
	Providers []string `json:"providers,omitempty"`
}
