package llm

import (
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func testLLMConfig() workorchconfig.LLMConfig {
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.AIProxy.APIKey = "secret-key"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "gpt-4o"}
	return cfg
}

func TestTargetConfigurationRevisionChangesOnSameLengthAPIKeyRotation(t *testing.T) {
	cfg := testLLMConfig()
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}
	first := TargetConfigurationRevision(cfg, target)
	cfg.AIProxy.APIKey = "secret-NEW" // same length
	second := TargetConfigurationRevision(cfg, target)
	if first == second {
		t.Fatalf("same-length API key rotation must change target configuration revision")
	}
	if !strings.HasPrefix(first, "target-config-sha256:") {
		t.Fatalf("revision format = %q", first)
	}
}

func TestCatalogConnectionRevisionChangesOnSameLengthAPIKeyRotation(t *testing.T) {
	cfg := testLLMConfig()
	first := CatalogConnectionRevision(cfg)
	cfg.AIProxy.APIKey = "secret-NEW"
	second := CatalogConnectionRevision(cfg)
	if first == second {
		t.Fatalf("same-length API key rotation must change catalog connection revision")
	}
}

func TestProfileFromProviderCeilingUsesSealedBudget(t *testing.T) {
	ceiling := runtimecontract.ProviderCeiling{
		Target:                   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 371712,
		CapabilityRevision:       "capability-sha256:abc",
		ConfigurationRevision:    "target-config-sha256:def",
	}
	profile, err := ProfileFromProviderCeiling(ceiling)
	if err != nil {
		t.Fatalf("ProfileFromProviderCeiling: %v", err)
	}
	if profile.MaxRequestBytes != 371712 || profile.MaxInputTokens != 92928 || profile.EffectiveMaxOutputTokens != 4096 {
		t.Fatalf("profile = %+v, want sealed budget fields", profile)
	}
	if profile.CapabilitySource != "sealed_ceiling" {
		t.Fatalf("capability source = %q", profile.CapabilitySource)
	}
}

func TestResolveProviderClientForSealedCeilingRejectsConfigDrift(t *testing.T) {
	cfg := testLLMConfig()
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}
	ceiling := runtimecontract.ProviderCeiling{
		Target:                   target,
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 371712,
		CapabilityRevision:       "capability-sha256:abc",
		ConfigurationRevision:    TargetConfigurationRevision(cfg, target),
	}
	if _, _, err := ResolveProviderClientForSealedCeiling(cfg, ceiling); err != nil {
		t.Fatalf("expected matching revision to succeed: %v", err)
	}
	cfg.AIProxy.APIKey = "rotated-key-1"
	_, _, err := ResolveProviderClientForSealedCeiling(cfg, ceiling)
	if err == nil {
		t.Fatal("expected configuration drift failure")
	}
	fail, ok := err.(llmmodels.ProviderModelFailure)
	if !ok || fail.Code != llmmodels.ProviderModelFailureConfigurationDrift {
		t.Fatalf("error = %v, want provider_configuration_drift failure", err)
	}
}

func TestResolveProviderModelTargetRequiresOperation(t *testing.T) {
	cfg := testLLMConfig()
	catalog := &llmmodels.ProviderModelCatalog{
		ProviderID: "ai-proxy",
		Models: []llmmodels.ProviderModelDescriptor{{
			Target:              runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embed-1"},
			ContextWindowTokens: 8192,
			MaxOutputTokens:     1024,
			Operations:          []string{"embeddings"},
			CapabilityReady:     true,
			CapabilityRevision:  "capability-sha256:embed",
		}},
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embed-1"}
	_, err := ResolveProviderModelTarget(cfg, catalog, target, "chat_completions")
	if err == nil {
		t.Fatal("expected operation_unsupported")
	}
	fail, ok := err.(llmmodels.ProviderModelFailure)
	if !ok || fail.Code != llmmodels.ProviderModelFailureOperationUnsupported {
		t.Fatalf("error = %v, want operation_unsupported", err)
	}
	if _, err := ResolveProviderModelTarget(cfg, catalog, target, "embeddings"); err != nil {
		t.Fatalf("embeddings resolve: %v", err)
	}
}

func TestCatalogConnectionRevisionIgnoresModelOverrides(t *testing.T) {
	cfg := testLLMConfig()
	first := CatalogConnectionRevision(cfg)
	cfg.AIProxy.ModelOverrides = map[string]workorchconfig.AIProxyModelOverrideConfig{
		"gpt-4o": {
			ContextWindowTokens: 200000,
			MaxOutputTokens:     8192,
			Operations:          []string{"chat_completions"},
		},
	}
	second := CatalogConnectionRevision(cfg)
	if first != second {
		t.Fatalf("catalog connection revision changed with model override: %q vs %q", first, second)
	}
	// API key still changes connection revision.
	cfg.AIProxy.APIKey = "another-secret-key"
	third := CatalogConnectionRevision(cfg)
	if third == first {
		t.Fatal("catalog connection revision did not change on API key rotation")
	}
}

func TestNormalizeCatalogOperationsRejectsAliasAndDuplicate(t *testing.T) {
	if ops := normalizeCatalogOperations([]string{"chat", "embeddings"}); ops != nil {
		t.Fatalf("alias operations = %v, want nil (strict reject)", ops)
	}
	if ops := normalizeCatalogOperations([]string{"chat_completions", "chat_completions"}); ops != nil {
		t.Fatalf("duplicate operations = %v, want nil", ops)
	}
	if ops := normalizeCatalogOperations([]string{"chat_completions", "embeddings"}); len(ops) != 2 {
		t.Fatalf("canonical operations = %v, want both kept", ops)
	}
	if ops := normalizeCatalogOperations(nil); ops != nil {
		t.Fatalf("empty operations = %v, want nil", ops)
	}
}

func TestResolveProviderModelTargetOverrideRequiresOperations(t *testing.T) {
	cfg := testLLMConfig()
	cfg.AIProxy.ModelOverrides = map[string]workorchconfig.AIProxyModelOverrideConfig{
		"gpt-4o": {ContextWindowTokens: 128000, MaxOutputTokens: 8192},
	}
	_, err := ResolveProviderModelTarget(cfg, nil, runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, "chat_completions")
	if err == nil {
		t.Fatal("expected incomplete override without operations")
	}
}
