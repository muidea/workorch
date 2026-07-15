package llm

import (
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
)

// RedactedProviderConfig returns a secret-free provider config for request audit payloads.
func RedactedProviderConfig(cfg llmmodels.ProviderConfig) llmmodels.RedactedProviderConfig {
	return llmproviderconfig.RedactedProviderConfig(cfg)
}

// RedactSecret masks secrets for logs and error bodies.
func RedactSecret(value string) string {
	return llmproviderconfig.RedactSecret(value)
}
