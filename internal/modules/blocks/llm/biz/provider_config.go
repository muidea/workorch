package llm

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const defaultProviderTimeout = 60 * time.Second
const defaultOpenAIBaseURL = "https://api.openai.com/v1"

func LoadProviderConfig(providerID string, values map[string]string) (llmmodels.ProviderConfig, error) {
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
	chatCompletions := true
	if raw := strings.TrimSpace(values[llmcommon.EnvChatCompletions]); raw != "" {
		parsed, parseErr := parseBool(raw)
		if parseErr != nil {
			return llmmodels.ProviderConfig{}, fmt.Errorf("%w: invalid %s", common.ErrInvalidArgument, llmcommon.EnvChatCompletions)
		}
		chatCompletions = parsed
	}
	streaming, err := parseBool(values[llmcommon.EnvStreaming])
	if err != nil {
		return llmmodels.ProviderConfig{}, fmt.Errorf("%w: invalid %s", common.ErrInvalidArgument, llmcommon.EnvStreaming)
	}

	cfg := llmmodels.ProviderConfig{
		ID:       normalizedProviderID(providerID),
		Provider: strings.TrimSpace(values[llmcommon.EnvProvider]),
		BaseURL:  strings.TrimSpace(values[llmcommon.EnvBaseURL]),
		APIKey:   strings.TrimSpace(values[llmcommon.EnvAPIKey]),
		Model:    strings.TrimSpace(values[llmcommon.EnvModel]),
		Timeout:  timeout,
		Disabled: disabled,
		Capabilities: llmmodels.ProviderCapabilities{
			ChatCompletions:   chatCompletions,
			Streaming:         streaming,
			NonChatOperations: normalizedOperations(values[llmcommon.EnvNonChatOperations]),
		},
	}
	return withAvailability(cfg), nil
}

func LoadProviderConfigs(defaultProviderID string, valueSets map[string]map[string]string) (string, map[string]llmmodels.ProviderConfig, error) {
	defaultProviderID = normalizedProviderID(defaultProviderID)
	if defaultProviderID == "" {
		defaultProviderID = llmcommon.DefaultProviderID
	}
	ret := make(map[string]llmmodels.ProviderConfig, len(valueSets))
	for id, values := range valueSets {
		cfg, err := LoadProviderConfig(id, values)
		if err != nil {
			return "", nil, err
		}
		ret[cfg.ID] = cfg
	}
	if len(ret) == 0 {
		cfg, err := LoadProviderConfig(defaultProviderID, nil)
		if err != nil {
			return "", nil, err
		}
		ret[cfg.ID] = cfg
	}
	return defaultProviderID, ret, nil
}

func RedactedProviderConfig(cfg llmmodels.ProviderConfig) llmmodels.RedactedProviderConfig {
	capabilities := effectiveProviderCapabilities(cfg.Capabilities)
	return llmmodels.RedactedProviderConfig{
		ID:           cfg.ID,
		Provider:     cfg.Provider,
		BaseURL:      cfg.BaseURL,
		Model:        cfg.Model,
		Timeout:      cfg.Timeout.String(),
		Disabled:     cfg.Disabled,
		Ready:        !cfg.Unavailable,
		Reason:       cfg.UnavailableReason,
		Capabilities: capabilities,
	}
}

func normalizedProviderID(providerID string) string {
	return strings.TrimSpace(providerID)
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

func normalizedOperations(raw string) []string {
	parts := strings.Split(raw, ",")
	ret := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		ret = append(ret, item)
	}
	return ret
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
