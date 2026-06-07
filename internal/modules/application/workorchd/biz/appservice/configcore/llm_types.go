package configcore

import llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"

type LLMConfiguration struct {
	DefaultProviderID          string                            `json:"defaultProviderId"`
	DefaultEmbeddingProviderID string                            `json:"defaultEmbeddingProviderId,omitempty"`
	ModelCapabilities          []LLMModelCapabilityConfiguration `json:"modelCapabilities,omitempty"`
	Providers                  []LLMProviderConfiguration        `json:"providers"`
	ProviderTemplates          []LLMProviderTemplate             `json:"providerTemplates"`
}

type LLMModelCapabilityConfiguration struct {
	ID                  string `json:"id"`
	Provider            string `json:"provider,omitempty"`
	ModelPattern        string `json:"modelPattern"`
	ContextWindowTokens int    `json:"contextWindowTokens"`
}

type LLMProviderConfiguration struct {
	ID                string                             `json:"id"`
	Provider          string                             `json:"provider,omitempty"`
	BaseURL           string                             `json:"baseUrl,omitempty"`
	Model             string                             `json:"model,omitempty"`
	Timeout           string                             `json:"timeout,omitempty"`
	Disabled          bool                               `json:"disabled"`
	Ready             bool                               `json:"ready"`
	Reason            string                             `json:"reason,omitempty"`
	Default           bool                               `json:"default"`
	APIKeyConfigured  bool                               `json:"apiKeyConfigured"`
	ChatCompletions   bool                               `json:"chatCompletions"`
	Streaming         bool                               `json:"streaming"`
	NonChatOperations []string                           `json:"nonChatOperations,omitempty"`
	FailoverEnabled   bool                               `json:"failoverEnabled"`
	FailoverProviders []string                           `json:"failoverProviders,omitempty"`
	InferenceProfile  llmmodels.ProviderInferenceProfile `json:"inferenceProfile,omitempty"`
	ContextProfile    llmmodels.ProviderContextProfile   `json:"contextProfile,omitempty"`
}

type LLMProviderTemplate struct {
	TemplateID        string   `json:"templateId"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	ID                string   `json:"id"`
	Provider          string   `json:"provider"`
	BaseURL           string   `json:"baseUrl,omitempty"`
	Model             string   `json:"model,omitempty"`
	Timeout           string   `json:"timeout,omitempty"`
	Disabled          bool     `json:"disabled"`
	ChatCompletions   bool     `json:"chatCompletions"`
	Streaming         bool     `json:"streaming"`
	NonChatOperations []string `json:"nonChatOperations,omitempty"`
}
