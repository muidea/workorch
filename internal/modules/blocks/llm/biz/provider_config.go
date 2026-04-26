package llm

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const defaultProviderTimeout = 60 * time.Second
const defaultOpenAIBaseURL = "https://api.openai.com/v1"

func LoadProviderConfigFromEnv() (llmmodels.ProviderConfig, error) {
	return LoadProviderConfig(envMap(os.Environ()))
}

func LoadProviderConfig(values map[string]string) (llmmodels.ProviderConfig, error) {
	timeout := defaultProviderTimeout
	if raw := strings.TrimSpace(values[llmcommon.EnvTimeout]); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return llmmodels.ProviderConfig{}, fmt.Errorf("%w: invalid %s", common.ErrInvalidArgument, llmcommon.EnvTimeout)
		}
		if parsed <= 0 {
			return llmmodels.ProviderConfig{}, fmt.Errorf("%w: %s must be positive", common.ErrInvalidArgument, llmcommon.EnvTimeout)
		}
		timeout = parsed
	}
	disabled, err := parseBool(values[llmcommon.EnvDisabled])
	if err != nil {
		return llmmodels.ProviderConfig{}, fmt.Errorf("%w: invalid %s", common.ErrInvalidArgument, llmcommon.EnvDisabled)
	}

	cfg := llmmodels.ProviderConfig{
		Provider: strings.TrimSpace(values[llmcommon.EnvProvider]),
		BaseURL:  strings.TrimSpace(values[llmcommon.EnvBaseURL]),
		APIKey:   strings.TrimSpace(values[llmcommon.EnvAPIKey]),
		Model:    strings.TrimSpace(values[llmcommon.EnvModel]),
		Timeout:  timeout,
		Disabled: disabled,
	}
	return withAvailability(cfg), nil
}

func RedactedProviderConfig(cfg llmmodels.ProviderConfig) llmmodels.RedactedProviderConfig {
	return llmmodels.RedactedProviderConfig{
		Provider: cfg.Provider,
		BaseURL:  cfg.BaseURL,
		Model:    cfg.Model,
		Timeout:  cfg.Timeout.String(),
		Disabled: cfg.Disabled,
		Ready:    !cfg.Unavailable,
		Reason:   cfg.UnavailableReason,
	}
}

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

func withAvailability(cfg llmmodels.ProviderConfig) llmmodels.ProviderConfig {
	switch {
	case cfg.Disabled:
		cfg.Unavailable = true
		cfg.UnavailableReason = "llm provider is disabled"
	case cfg.Provider == "":
		cfg.Unavailable = true
		cfg.UnavailableReason = "llm provider is not configured"
	case !isSupportedProvider(cfg.Provider):
		cfg.Unavailable = true
		cfg.UnavailableReason = "llm provider is unsupported"
	case cfg.Provider == llmcommon.ProviderCompatible && cfg.BaseURL == "":
		cfg.Unavailable = true
		cfg.UnavailableReason = "llm compatible provider base url is not configured"
	case cfg.Model == "":
		cfg.Unavailable = true
		cfg.UnavailableReason = "llm model is not configured"
	case cfg.APIKey == "":
		cfg.Unavailable = true
		cfg.UnavailableReason = "llm api key is not configured"
	}
	if !cfg.Unavailable && cfg.Provider == llmcommon.ProviderOpenAI && cfg.BaseURL == "" {
		cfg.BaseURL = defaultOpenAIBaseURL
	}
	return cfg
}

func isSupportedProvider(provider string) bool {
	switch provider {
	case llmcommon.ProviderOpenAI, llmcommon.ProviderCompatible:
		return true
	default:
		return false
	}
}

func parseBool(raw string) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, nil
	}
	return strconv.ParseBool(raw)
}

func envMap(values []string) map[string]string {
	ret := make(map[string]string, len(values))
	for _, item := range values {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		ret[key] = value
	}
	return ret
}
