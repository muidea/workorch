package configcore

type UpdateLLMConfigurationCommand struct {
	// The configuration plane owns one provider only: ai-proxy. Endpoint and
	// transport settings remain file-configured and are deliberately absent.
	APIKey                  *string                        `json:"apiKey,omitempty"`
	DefaultChatModelID      *string                        `json:"defaultChatModelId,omitempty"`
	DefaultEmbeddingModelID *string                        `json:"defaultEmbeddingModelId,omitempty"`
	ClearDefaultEmbedding   bool                           `json:"clearDefaultEmbedding,omitempty"`
	Budget                  *UpdateLLMContextBudgetCommand `json:"budget,omitempty"`
}

type UpdateLLMContextBudgetCommand struct {
	ContextUtilizationRatio *float64 `json:"contextUtilizationRatio,omitempty"`
	EstimatedTokensPerByte  *float64 `json:"estimatedTokensPerByte,omitempty"`
}
