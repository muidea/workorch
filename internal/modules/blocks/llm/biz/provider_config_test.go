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

	redacted := RedactedProviderConfig(cfg)
	if !redacted.Ready || redacted.Model != "local-model" || !redacted.Capabilities.ChatCompletions {
		t.Fatalf("redacted config = %+v, want ready local-model", redacted)
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
