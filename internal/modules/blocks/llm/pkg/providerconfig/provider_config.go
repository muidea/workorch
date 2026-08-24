// Package providerconfig contains redaction helpers used by the request audit path.
package providerconfig

import (
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

// RedactedProviderConfig returns a secret-free provider config for request audit payloads.
func RedactedProviderConfig(cfg llmmodels.ProviderConfig) llmmodels.RedactedProviderConfig {
	return llmmodels.RedactedProviderConfig{
		ID:               cfg.ID,
		APISurface:       cfg.APISurface,
		BaseURL:          cfg.BaseURL,
		Model:            cfg.Model,
		Timeout:          cfg.Timeout.String(),
		Disabled:         cfg.Disabled,
		Ready:            !cfg.Unavailable,
		Reason:           cfg.UnavailableReason,
		Capabilities:     cfg.Capabilities,
		InferenceProfile: cfg.InferenceProfile,
		ContextProfile:   cfg.ContextProfile,
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
