// Package providerconfig retains only redaction helpers used by the request audit path.
// Legacy multi-provider LoadProviderConfig / contextProfileMode / conservative default
// resolution was removed by the ai-proxy primary cutover; production clients are built
// from workorchconfig.LLMConfig via ResolveProviderClient.
package providerconfig

import (
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

// RedactedProviderConfig returns a secret-free provider config for request audit payloads.
func RedactedProviderConfig(cfg llmmodels.ProviderConfig) llmmodels.RedactedProviderConfig {
	capabilities := effectiveProviderCapabilities(cfg.Capabilities)
	return llmmodels.RedactedProviderConfig{
		ID:               cfg.ID,
		Provider:         cfg.Provider,
		APISurface:       cfg.APISurface,
		BaseURL:          cfg.BaseURL,
		Model:            cfg.Model,
		Timeout:          cfg.Timeout.String(),
		Disabled:         cfg.Disabled,
		Ready:            !cfg.Unavailable,
		Reason:           cfg.UnavailableReason,
		Capabilities:     capabilities,
		CacheCapability:  cfg.CacheCapability,
		InferenceProfile: cfg.InferenceProfile,
		ContextProfile:   cfg.ContextProfile,
		Failover: llmmodels.ProviderFailoverConfig{
			Enabled:   cfg.FailoverEnabled,
			Providers: append([]string(nil), cfg.FailoverProviders...),
		},
	}
}

// RedactSecret masks secrets for logs and error bodies. Short secrets are fully masked.
func RedactSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func effectiveProviderCapabilities(capabilities llmmodels.ProviderCapabilities) llmmodels.ProviderCapabilities {
	if !capabilities.ChatCompletions && !capabilities.Streaming && len(capabilities.NonChatOperations) == 0 {
		capabilities.ChatCompletions = true
	}
	return capabilities
}
