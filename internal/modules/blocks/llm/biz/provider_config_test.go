package llm

import (
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestRedactSecret(t *testing.T) {
	if got := RedactSecret("abc"); got != "********" {
		t.Fatalf("short redaction = %q, want ********", got)
	}
	if got := RedactSecret("1234567890"); got != "1234...7890" {
		t.Fatalf("long redaction = %q, want 1234...7890", got)
	}
}

func TestRedactedProviderConfigOmitsSecrets(t *testing.T) {
	cfg := llmmodels.ProviderConfig{
		ID: "ai-proxy", Provider: llmcommon.ProviderCompatible, APISurface: llmcommon.ProviderSurfaceOpenAICompatibleHTTP,
		BaseURL: "http://127.0.0.1:8080", APIKey: "super-secret-key", Model: "gpt-4o", Timeout: 30 * time.Second,
		Capabilities: llmmodels.ProviderCapabilities{ChatCompletions: true},
		ContextProfile: llmmodels.ProviderContextProfile{
			CapabilitySource: "ai_proxy_catalog", ContextWindowTokens: 128000, MaxInputTokens: 120000,
		},
	}
	redacted := RedactedProviderConfig(cfg)
	if !redacted.Ready || redacted.Model != "gpt-4o" || redacted.ID != "ai-proxy" {
		t.Fatalf("redacted = %+v, want ready ai-proxy/gpt-4o", redacted)
	}
	if redacted.ContextProfile.CapabilitySource != "ai_proxy_catalog" {
		t.Fatalf("capability source = %q, want sealed/catalog source preserved", redacted.ContextProfile.CapabilitySource)
	}
}
