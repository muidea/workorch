package llm

import (
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResolveProviderModelTargetUsesCatalogOutputCapabilityWithoutLocalCap(t *testing.T) {
	cfg := testCatalogOutputCeilingConfig("large-output-model")
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "large-output-model"}
	profile, err := ResolveProviderModelTarget(cfg, testCatalogOutputCeiling(target), target, workorchconfig.LLMOperationChatCompletions)
	if err != nil {
		t.Fatalf("ResolveProviderModelTarget: %v", err)
	}
	if profile.EffectiveMaxOutputTokens != 65_536 || profile.ReservedOutputTokens != 65_536 {
		t.Fatalf("profile output ceiling = %+v, want catalog capability", profile)
	}
}

func TestResolveProviderModelTargetHonorsExplicitLocalOutputCap(t *testing.T) {
	cfg := testCatalogOutputCeilingConfig("capped-model")
	cfg.Budget.DefaultMaxOutputTokens = 4096
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "capped-model"}
	profile, err := ResolveProviderModelTarget(cfg, testCatalogOutputCeiling(target), target, workorchconfig.LLMOperationChatCompletions)
	if err != nil {
		t.Fatalf("ResolveProviderModelTarget: %v", err)
	}
	if profile.EffectiveMaxOutputTokens != 4096 {
		t.Fatalf("profile output ceiling = %d, want explicit cap", profile.EffectiveMaxOutputTokens)
	}
}

func testCatalogOutputCeilingConfig(modelID string) workorchconfig.LLMConfig {
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://provider.example.test"
	cfg.AIProxy.APIKey = "test-key"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: modelID}
	return cfg
}

func testCatalogOutputCeiling(target runtimecontract.ProviderModelTarget) *llmmodels.ProviderModelCatalog {
	return &llmmodels.ProviderModelCatalog{ProviderID: "ai-proxy", Models: []llmmodels.ProviderModelDescriptor{{
		Target: target, ContextWindowTokens: 1_000_000, MaxOutputTokens: 65_536,
		Operations: []string{workorchconfig.LLMOperationChatCompletions}, CapabilityReady: true,
		CapabilityRevision: "capability-sha256:test",
	}}}
}
