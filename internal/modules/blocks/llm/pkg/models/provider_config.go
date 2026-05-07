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
	ContextProfile    ProviderContextProfile
	Unavailable       bool
	UnavailableReason string
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
	ID             string                 `json:"id,omitempty"`
	Provider       string                 `json:"provider,omitempty"`
	BaseURL        string                 `json:"baseUrl,omitempty"`
	Model          string                 `json:"model,omitempty"`
	Timeout        string                 `json:"timeout,omitempty"`
	Disabled       bool                   `json:"disabled"`
	Ready          bool                   `json:"ready"`
	Reason         string                 `json:"reason,omitempty"`
	Capabilities   ProviderCapabilities   `json:"capabilities"`
	ContextProfile ProviderContextProfile `json:"contextProfile,omitempty"`
}
