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

func TestResolveProviderModelTargetDerivesRequestBytesFromSelectedModel(t *testing.T) {
	cfg := testCatalogOutputCeilingConfig("large-context-model")
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "large-context-model"}
	catalog := testCatalogOutputCeiling(target)
	catalog.Models[0].ContextWindowTokens = 2_000_000

	profile, err := ResolveProviderModelTarget(cfg, catalog, target, workorchconfig.LLMOperationChatCompletions)
	if err != nil {
		t.Fatalf("ResolveProviderModelTarget: %v", err)
	}
	const wantMaxInputTokens = 1_450_848
	const wantMaxRequestBytes = 5_803_392
	if profile.MaxInputTokens != wantMaxInputTokens || profile.MaxRequestBytes != wantMaxRequestBytes {
		t.Fatalf("profile input ceiling = %+v, want tokens=%d bytes=%d derived from model capability", profile, wantMaxInputTokens, wantMaxRequestBytes)
	}
}

func TestResolveEmbeddingTargetDoesNotRequireChatTokenCapabilities(t *testing.T) {
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8090"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "gpt-5.5"}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "intfloat/multilingual-e5-small"}
	catalog := &llmmodels.ProviderModelCatalog{ProviderID: "ai-proxy", Models: []llmmodels.ProviderModelDescriptor{{Target: target, Operations: []string{workorchconfig.LLMOperationEmbeddings}, CapabilityReady: true, CapabilityRevision: "embedding-capability"}}}
	profile, err := ResolveProviderModelTarget(cfg, catalog, target, workorchconfig.LLMOperationEmbeddings)
	if err != nil {
		t.Fatalf("ResolveProviderModelTarget: %v", err)
	}
	if profile.ProviderMaxOutputTokens != 0 || profile.ContextWindowTokens != 0 {
		t.Fatalf("embedding profile carries chat token capabilities: %+v", profile)
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
