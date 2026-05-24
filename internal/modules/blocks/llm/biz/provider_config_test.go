package llm

import (
	"errors"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestLoadProviderConfigReportsUnavailableWhenMissing(t *testing.T) {
	cfg, err := LoadProviderConfig("local", map[string]string{})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if !cfg.Unavailable || cfg.UnavailableReason != "llm provider is not configured" {
		t.Fatalf("config = %+v, want provider not configured", cfg)
	}
}

func TestLoadProviderConfigAcceptsCompatibleProvider(t *testing.T) {
	cfg, err := LoadProviderConfig("local", map[string]string{
		llmcommon.EnvProvider: llmcommon.ProviderCompatible,
		llmcommon.EnvBaseURL:  "http://127.0.0.1:11434/v1",
		llmcommon.EnvAPIKey:   "test-secret-value",
		llmcommon.EnvModel:    "local-model",
		llmcommon.EnvTimeout:  "5s",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.ID != "local" || cfg.Unavailable {
		t.Fatalf("config unavailable = %+v, want ready", cfg)
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", cfg.Timeout)
	}
	if !cfg.Capabilities.ChatCompletions || cfg.Capabilities.Streaming || len(cfg.Capabilities.NonChatOperations) != 0 {
		t.Fatalf("capabilities = %+v, want default chat completions only", cfg.Capabilities)
	}
	if cfg.ContextProfile.ContextWindowTokens != defaultContextWindowTokens ||
		cfg.ContextProfile.MaxInputTokens != defaultContextWindowTokens-defaultReservedOutputTokens ||
		cfg.ContextProfile.ReservedOutputTokens != defaultReservedOutputTokens ||
		cfg.ContextProfile.SafetyRatio != defaultContextSafetyRatio ||
		cfg.ContextProfile.ContextUtilizationRatio != defaultContextUtilizationRatio ||
		cfg.ContextProfile.BytesHardLimit != defaultContextBytesHardLimit ||
		cfg.ContextProfile.MaxRequestBytes != defaultContextMaxRequestBytes ||
		cfg.ContextProfile.EstimatedTokensPerByte != defaultEstimatedTokensPerByte {
		t.Fatalf("context profile = %+v, want normalized defaults", cfg.ContextProfile)
	}

	redacted := RedactedProviderConfig(cfg)
	if !redacted.Ready || redacted.Model != "local-model" || !redacted.Capabilities.ChatCompletions {
		t.Fatalf("redacted config = %+v, want ready local-model", redacted)
	}
	if redacted.ContextProfile.ContextWindowTokens != defaultContextWindowTokens {
		t.Fatalf("redacted context profile = %+v, want exported profile", redacted.ContextProfile)
	}
}

func TestLoadProviderConfigParsesDeclaredCapabilities(t *testing.T) {
	cfg, err := LoadProviderConfig("local", map[string]string{
		llmcommon.EnvProvider:          llmcommon.ProviderCompatible,
		llmcommon.EnvBaseURL:           "http://127.0.0.1:11434/v1",
		llmcommon.EnvAPIKey:            "test-secret-value",
		llmcommon.EnvModel:             "local-model",
		llmcommon.EnvChatCompletions:   "false",
		llmcommon.EnvStreaming:         "true",
		llmcommon.EnvNonChatOperations: "embeddings, responses, embeddings",
		llmcommon.EnvFailoverEnabled:   "true",
		llmcommon.EnvFailoverProviders: "backup-a, backup-b, backup-a",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.Capabilities.ChatCompletions || !cfg.Capabilities.Streaming ||
		len(cfg.Capabilities.NonChatOperations) != 2 ||
		cfg.Capabilities.NonChatOperations[0] != "embeddings" ||
		cfg.Capabilities.NonChatOperations[1] != "responses" {
		t.Fatalf("capabilities = %+v, want declared streaming and non-chat operations", cfg.Capabilities)
	}
	if !cfg.FailoverEnabled || len(cfg.FailoverProviders) != 2 ||
		cfg.FailoverProviders[0] != "backup-a" || cfg.FailoverProviders[1] != "backup-b" {
		t.Fatalf("failover = enabled:%v providers:%v, want normalized failover settings", cfg.FailoverEnabled, cfg.FailoverProviders)
	}
	redacted := RedactedProviderConfig(cfg)
	if !redacted.Failover.Enabled || len(redacted.Failover.Providers) != 2 {
		t.Fatalf("redacted failover = %+v, want exported failover config", redacted.Failover)
	}
}

func TestLoadProviderConfigRequiresCompatibleProviderBaseURL(t *testing.T) {
	cfg, err := LoadProviderConfig("local", map[string]string{
		llmcommon.EnvProvider: llmcommon.ProviderCompatible,
		llmcommon.EnvAPIKey:   "test-secret-value",
		llmcommon.EnvModel:    "local-model",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if !cfg.Unavailable || cfg.UnavailableReason != "llm compatible provider base url is not configured" {
		t.Fatalf("config = %+v, want compatible base url unavailable", cfg)
	}
}

func TestLoadProviderConfigDefaultsOpenAIBaseURL(t *testing.T) {
	cfg, err := LoadProviderConfig("reasoning", map[string]string{
		llmcommon.EnvProvider: llmcommon.ProviderOpenAI,
		llmcommon.EnvAPIKey:   "test-secret-value",
		llmcommon.EnvModel:    "gpt-test",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.Unavailable || cfg.BaseURL != defaultOpenAIBaseURL {
		t.Fatalf("config = %+v, want ready openai default base url", cfg)
	}
}

func TestLoadProviderConfigDefaultsAnthropicBaseURL(t *testing.T) {
	cfg, err := LoadProviderConfig("claude", map[string]string{
		llmcommon.EnvProvider: llmcommon.ProviderAnthropic,
		llmcommon.EnvAPIKey:   "test-secret-value",
		llmcommon.EnvModel:    "claude-test",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.Unavailable || cfg.BaseURL != defaultAnthropicBaseURL {
		t.Fatalf("config = %+v, want ready anthropic default base url", cfg)
	}
}

func TestLoadProviderConfigRejectsInvalidTimeout(t *testing.T) {
	_, err := LoadProviderConfig("local", map[string]string{
		llmcommon.EnvTimeout: "bad",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("load invalid timeout err = %v, want invalid argument", err)
	}
}

func TestLoadProviderConfigParsesContextProfile(t *testing.T) {
	cfg, err := LoadProviderConfig("reasoning", map[string]string{
		llmcommon.EnvProvider:           llmcommon.ProviderOpenAI,
		llmcommon.EnvAPIKey:             "reasoning-secret",
		llmcommon.EnvModel:              "reasoning-model",
		llmcommon.EnvContextProfileMode: contextProfileModeManual,
		llmcommon.EnvContextWindow:      "128000",
		llmcommon.EnvMaxInputTokens:     "64000",
		llmcommon.EnvReservedOutput:     "8000",
		llmcommon.EnvSafetyRatio:        "0.85",
		llmcommon.EnvContextUtilization: "0.65",
		llmcommon.EnvBytesHardLimit:     "262144",
		llmcommon.EnvMaxRequestBytes:    "524288",
		llmcommon.EnvTokensPerByte:      "0.22",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.ContextProfile.ProfileMode != contextProfileModeManual ||
		cfg.ContextProfile.ContextWindowTokens != 128000 ||
		cfg.ContextProfile.MaxInputTokens != 64000 ||
		cfg.ContextProfile.ReservedOutputTokens != 8000 ||
		cfg.ContextProfile.SafetyRatio != 0.85 ||
		cfg.ContextProfile.ContextUtilizationRatio != 0.65 ||
		cfg.ContextProfile.BytesHardLimit != 262144 ||
		cfg.ContextProfile.MaxRequestBytes != 524288 ||
		cfg.ContextProfile.EstimatedTokensPerByte != 0.22 {
		t.Fatalf("context profile = %+v, want parsed values", cfg.ContextProfile)
	}
}

func TestLoadProviderConfigParsesInferenceProfile(t *testing.T) {
	cfg, err := LoadProviderConfig("reasoning", map[string]string{
		llmcommon.EnvProvider:        llmcommon.ProviderOpenAI,
		llmcommon.EnvAPIKey:          "reasoning-secret",
		llmcommon.EnvModel:           "reasoning-model",
		llmcommon.EnvTemperature:     "0",
		llmcommon.EnvMaxOutputTokens: "65536",
		llmcommon.EnvReasoningEffort: "HIGH",
		llmcommon.EnvThinkingBudget:  "32768",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.InferenceProfile.Temperature == nil ||
		*cfg.InferenceProfile.Temperature != 0 ||
		cfg.InferenceProfile.MaxOutputTokens != 65536 ||
		cfg.InferenceProfile.ReasoningEffort != "high" ||
		cfg.InferenceProfile.ThinkingBudgetTokens != 32768 {
		t.Fatalf("inference profile = %+v, want parsed reasoning profile", cfg.InferenceProfile)
	}
	redacted := RedactedProviderConfig(cfg)
	if redacted.InferenceProfile.ReasoningEffort != "high" ||
		redacted.InferenceProfile.ThinkingBudgetTokens != 32768 {
		t.Fatalf("redacted inference profile = %+v, want exported profile", redacted.InferenceProfile)
	}
}

func TestLoadProviderConfigAutoUsesConfiguredModelRegistryCapability(t *testing.T) {
	cfg, err := LoadProviderConfig("large-context", map[string]string{
		llmcommon.EnvProvider:                llmcommon.ProviderCompatible,
		llmcommon.EnvBaseURL:                 "http://127.0.0.1:8090/v1",
		llmcommon.EnvAPIKey:                  "large-context-secret",
		llmcommon.EnvModel:                   "configured-large-context-model",
		llmcommon.EnvContextProfileMode:      contextProfileModeAuto,
		llmcommon.EnvContextCapabilitySource: contextCapabilitySourceModelRegistry,
		llmcommon.EnvContextWindow:           "1000000",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.ContextProfile.ProfileMode != contextProfileModeAuto ||
		cfg.ContextProfile.CapabilitySource != contextCapabilitySourceModelRegistry ||
		cfg.ContextProfile.ContextWindowTokens != 1_000_000 ||
		cfg.ContextProfile.ReservedOutputTokens != 62_500 ||
		cfg.ContextProfile.MaxInputTokens != 703_125 ||
		cfg.ContextProfile.BytesHardLimit != 2_812_500 {
		t.Fatalf("context profile = %+v, want auto 1M configured model budget", cfg.ContextProfile)
	}
}

func TestLoadProviderConfigAutoIgnoresExplicitBudgetOverrides(t *testing.T) {
	cfg, err := LoadProviderConfig("deepseek", map[string]string{
		llmcommon.EnvProvider:                llmcommon.ProviderCompatible,
		llmcommon.EnvBaseURL:                 "https://api.deepseek.com",
		llmcommon.EnvAPIKey:                  "deepseek-secret",
		llmcommon.EnvModel:                   "configured-large-context-model",
		llmcommon.EnvContextProfileMode:      contextProfileModeAuto,
		llmcommon.EnvContextCapabilitySource: contextCapabilitySourceModelRegistry,
		llmcommon.EnvContextWindow:           "1000000",
		llmcommon.EnvMaxInputTokens:          "12345",
		llmcommon.EnvBytesHardLimit:          "222222",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.ContextProfile.ContextWindowTokens != 1_000_000 ||
		cfg.ContextProfile.ReservedOutputTokens != 62_500 ||
		cfg.ContextProfile.MaxInputTokens != 703_125 ||
		cfg.ContextProfile.BytesHardLimit != 2_812_500 {
		t.Fatalf("context profile = %+v, want auto budget derived from context window", cfg.ContextProfile)
	}
}

func TestLoadProviderConfigAutoDerivesLargeBudgetFromConfiguredContextWindow(t *testing.T) {
	cfg, err := LoadProviderConfig("minimax", map[string]string{
		llmcommon.EnvProvider:           llmcommon.ProviderOpenAI,
		llmcommon.EnvAPIKey:             "minimax-secret",
		llmcommon.EnvModel:              "MiniMax-M2.7-highspeed",
		llmcommon.EnvContextProfileMode: contextProfileModeAuto,
		llmcommon.EnvContextWindow:      "204800",
		llmcommon.EnvMaxInputTokens:     "159040",
		llmcommon.EnvReservedOutput:     "4096",
		llmcommon.EnvBytesHardLimit:     "98304",
		llmcommon.EnvMaxRequestBytes:    "4194304",
		llmcommon.EnvTokensPerByte:      "0.25",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.ContextProfile.ContextWindowTokens != 204800 ||
		cfg.ContextProfile.ReservedOutputTokens != 12800 ||
		cfg.ContextProfile.MaxInputTokens != 144000 ||
		cfg.ContextProfile.BytesHardLimit != 576000 {
		t.Fatalf("context profile = %+v, want auto MiniMax budget derived from 200k window", cfg.ContextProfile)
	}
}

func TestLoadProviderConfigRejectsInvalidContextProfile(t *testing.T) {
	_, err := LoadProviderConfig("reasoning", map[string]string{
		llmcommon.EnvProvider:      llmcommon.ProviderOpenAI,
		llmcommon.EnvAPIKey:        "reasoning-secret",
		llmcommon.EnvModel:         "reasoning-model",
		llmcommon.EnvContextWindow: "bad",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("load invalid context profile err = %v, want invalid argument", err)
	}
}

func TestLoadProviderConfigsKeepsMultipleProviders(t *testing.T) {
	defaultProviderID, configs, err := LoadProviderConfigs("reasoning", map[string]map[string]string{
		"local": {
			llmcommon.EnvProvider: llmcommon.ProviderCompatible,
			llmcommon.EnvBaseURL:  "http://127.0.0.1:11434/v1",
			llmcommon.EnvAPIKey:   "local-secret",
			llmcommon.EnvModel:    "local-model",
		},
		"reasoning": {
			llmcommon.EnvProvider: llmcommon.ProviderOpenAI,
			llmcommon.EnvAPIKey:   "reasoning-secret",
			llmcommon.EnvModel:    "reasoning-model",
		},
	})
	if err != nil {
		t.Fatalf("load provider configs: %v", err)
	}
	if defaultProviderID != "reasoning" || len(configs) != 2 || configs["local"].Model != "local-model" || configs["reasoning"].Model != "reasoning-model" {
		t.Fatalf("default=%q configs=%+v, want two provider configs", defaultProviderID, configs)
	}
}

func TestRedactSecret(t *testing.T) {
	if got := RedactSecret("abc"); got != "********" {
		t.Fatalf("short redaction = %q, want ********", got)
	}
	if got := RedactSecret("1234567890"); got != "1234...7890" {
		t.Fatalf("long redaction = %q, want 1234...7890", got)
	}
}
