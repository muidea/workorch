package llm

import (
	"testing"
	"time"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResolveProviderClientSeparatesResponseIdleAndHardTimeout(t *testing.T) {
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://provider.example.test"
	cfg.AIProxy.APIKey = "test-key"
	cfg.AIProxy.Timeout = "37s"
	target := runtimecontract.ProviderModelTarget{ProviderID: workorchconfig.BuiltinAIProxyProviderID, ModelID: "model-1"}
	profile := llmmodels.ResolvedProviderModelProfile{Target: target, EffectiveMaxOutputTokens: 1024}

	resolved, err := ResolveProviderClient(cfg, target, profile)
	if err != nil {
		t.Fatalf("ResolveProviderClient: %v", err)
	}
	if resolved.Timeout != 37*time.Second || resolved.HardTimeout != workorchconfig.MaxAIProxyTimeout {
		t.Fatalf("resolved provider timeouts = idle %s hard %s", resolved.Timeout, resolved.HardTimeout)
	}
}

func TestResolveProviderModelTargetUsesCatalogOutputCapabilityWithoutLocalCap(t *testing.T) {
	cfg := testCatalogOutputCeilingConfig("large-output-model")
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "large-output-model"}
	profile, err := ResolveProviderModelTarget(cfg, testCatalogOutputCeiling(target), target, workorchconfig.LLMOperationResponses)
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

	profile, err := ResolveProviderModelTarget(cfg, catalog, target, workorchconfig.LLMOperationResponses)
	if err != nil {
		t.Fatalf("ResolveProviderModelTarget: %v", err)
	}
	const wantMaxInputTokens = 1_450_848
	const wantMaxRequestBytes = 5_803_392
	if profile.MaxInputTokens != wantMaxInputTokens || profile.MaxRequestBytes != wantMaxRequestBytes {
		t.Fatalf("profile input ceiling = %+v, want tokens=%d bytes=%d derived from model capability", profile, wantMaxInputTokens, wantMaxRequestBytes)
	}
}

func TestResolveEmbeddingTargetDerivesByteCapacityWithoutOutputCeiling(t *testing.T) {
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8090"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "gpt-5.5"}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "intfloat/multilingual-e5-small"}
	catalog := &llmmodels.ProviderModelCatalog{ProviderID: "ai-proxy", Models: []llmmodels.ProviderModelDescriptor{{Target: target, ContextWindowTokens: 512, Operations: []string{workorchconfig.LLMOperationEmbeddings}, CapabilityReady: true, CapabilityRevision: "embedding-capability"}}}
	profile, err := ResolveProviderModelTarget(cfg, catalog, target, workorchconfig.LLMOperationEmbeddings)
	if err != nil {
		t.Fatalf("ResolveProviderModelTarget: %v", err)
	}
	if profile.ProviderMaxOutputTokens != 0 || profile.ContextWindowTokens != 512 || profile.MaxInputTokens != 384 || profile.MaxRequestBytes != 1536 {
		t.Fatalf("embedding profile capacity = %+v", profile)
	}
}

func TestResolveEmbeddingTargetRejectsInvalidInputCapacityEvenIfMarkedReady(t *testing.T) {
	for _, capacity := range []int{-1} {
		cfg := testCatalogOutputCeilingConfig("chat-model")
		target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embedding-model"}
		catalog := &llmmodels.ProviderModelCatalog{ProviderID: "ai-proxy", Models: []llmmodels.ProviderModelDescriptor{{
			Target: target, ContextWindowTokens: capacity, CapabilityReady: true,
			Operations: []string{workorchconfig.LLMOperationEmbeddings},
		}}}
		if _, err := ResolveProviderModelTarget(cfg, catalog, target, workorchconfig.LLMOperationEmbeddings); err == nil {
			t.Fatalf("invalid embedding input capacity %d must fail closed", capacity)
		}
	}
}

func TestEmbeddingLocalBudgetBoundsUnknownAndLargeKnownCapacity(t *testing.T) {
	cfg := testCatalogOutputCeilingConfig("chat-model")
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embedding-model"}
	for _, capacity := range []int{0, 1_000_000} {
		catalog := &llmmodels.ProviderModelCatalog{ProviderID: "ai-proxy", Models: []llmmodels.ProviderModelDescriptor{{
			Target: target, ContextWindowTokens: capacity, EmbeddingReady: true,
			Operations: []string{workorchconfig.LLMOperationEmbeddings}, CapabilityRevision: "catalog-original",
		}}}
		profile, err := ResolveProviderModelTarget(cfg, catalog, target, workorchconfig.LLMOperationEmbeddings)
		if err != nil || profile.MaxRequestBytes != embeddingMaxInputBytes || profile.ContextWindowTokens != capacity || profile.ProviderMaxOutputTokens != 0 || profile.CapabilityRevision == "catalog-original" {
			t.Fatalf("capacity %d profile=%+v err=%v", capacity, profile, err)
		}
		if capacity == 0 && profile.MaxInputTokens != 0 {
			t.Fatal("unknown token capacity invented")
		}
		client, err := ResolveProviderClient(cfg, target, profile)
		if err != nil || client.ContextProfile.MaxRequestBytes != profile.MaxRequestBytes || client.ContextProfile.ContextWindowTokens != capacity {
			t.Fatalf("embedding client mapping=%+v err=%v", client.ContextProfile, err)
		}
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
		Operations: []string{workorchconfig.LLMOperationResponses}, CapabilityReady: true,
		CapabilityRevision: "capability-sha256:test",
	}}}
}
