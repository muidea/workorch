package models

import (
	"time"

	"github.com/muidea/workorch/internal/pkg/llmaudit"
)

type ProviderConfig struct {
	ID                string
	APISurface        string
	BaseURL           string
	APIKey            string
	Model             string
	Timeout           time.Duration
	Disabled          bool
	Capabilities      ProviderCapabilities
	InferenceProfile  ProviderInferenceProfile
	ContextProfile    ProviderContextProfile
	Unavailable       bool
	UnavailableReason string
}

type ProviderInferenceProfile struct {
	Temperature          *float64 `json:"temperature,omitempty"`
	MaxOutputTokens      int      `json:"maxOutputTokens,omitempty"`
	ThinkingBudgetTokens int      `json:"thinkingBudgetTokens,omitempty"`
}

type ProviderCapabilities struct {
	Responses          bool     `json:"responses,omitempty"`
	Streaming          bool     `json:"streaming,omitempty"`
	JSONSchemaResponse bool     `json:"jsonSchemaResponse,omitempty"`
	NonChatOperations  []string `json:"nonChatOperations,omitempty"`
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
	ProviderID           string                        `json:"providerId"`
	Model                string                        `json:"model"`
	Input                string                        `json:"input,omitempty"`
	Vector               []float64                     `json:"vector"`
	Usage                *llmaudit.ProviderUsage       `json:"usage,omitempty"`
	PhysicalResponseMode llmaudit.PhysicalResponseMode `json:"physicalResponseMode"`
	// ProviderSubmitted is the adapter-owned HTTP dispatch fact. It is
	// deliberately independent from the serialized request byte count.
	ProviderSubmitted bool `json:"-"`
	RequestBytes      int  `json:"-"`
	ResponseBytes     int  `json:"-"`
}

type RedactedProviderConfig struct {
	ID               string                   `json:"id,omitempty"`
	APISurface       string                   `json:"apiSurface,omitempty"`
	BaseURL          string                   `json:"baseUrl,omitempty"`
	Model            string                   `json:"model,omitempty"`
	Timeout          string                   `json:"timeout,omitempty"`
	Disabled         bool                     `json:"disabled"`
	Ready            bool                     `json:"ready"`
	Reason           string                   `json:"reason,omitempty"`
	Capabilities     ProviderCapabilities     `json:"capabilities"`
	InferenceProfile ProviderInferenceProfile `json:"inferenceProfile,omitempty"`
	ContextProfile   ProviderContextProfile   `json:"contextProfile,omitempty"`
}
