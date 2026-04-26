package llm

import (
	"errors"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestLoadProviderConfigReportsUnavailableWhenMissing(t *testing.T) {
	cfg, err := LoadProviderConfig(map[string]string{})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if !cfg.Unavailable || cfg.UnavailableReason != "llm provider is not configured" {
		t.Fatalf("config = %+v, want provider not configured", cfg)
	}
}

func TestLoadProviderConfigAcceptsCompatibleProvider(t *testing.T) {
	cfg, err := LoadProviderConfig(map[string]string{
		llmcommon.EnvProvider: llmcommon.ProviderCompatible,
		llmcommon.EnvBaseURL:  "http://127.0.0.1:11434/v1",
		llmcommon.EnvAPIKey:   "test-secret-value",
		llmcommon.EnvModel:    "local-model",
		llmcommon.EnvTimeout:  "5s",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.Unavailable {
		t.Fatalf("config unavailable = %+v, want ready", cfg)
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", cfg.Timeout)
	}

	redacted := RedactedProviderConfig(cfg)
	if !redacted.Ready || redacted.Model != "local-model" {
		t.Fatalf("redacted config = %+v, want ready local-model", redacted)
	}
}

func TestLoadProviderConfigRequiresCompatibleProviderBaseURL(t *testing.T) {
	cfg, err := LoadProviderConfig(map[string]string{
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
	cfg, err := LoadProviderConfig(map[string]string{
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

func TestLoadProviderConfigRejectsInvalidTimeout(t *testing.T) {
	_, err := LoadProviderConfig(map[string]string{
		llmcommon.EnvTimeout: "bad",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("load invalid timeout err = %v, want invalid argument", err)
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
