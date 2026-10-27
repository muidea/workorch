package events

import (
	"math"
	"testing"

	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationProviderBudgetIsBoundedCapacityNotAuthority(t *testing.T) {
	fixture := func() PreparationProviderBudget {
		return PreparationProviderBudget{Version: PreparationProviderBudgetVersion, Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "exact"}, ContextWindowTokens: 32000, ProviderMaxOutputTokens: 4000, EffectiveMaxOutputTokens: 4000, MaxInputTokens: 20000, MaxRequestBytes: 80000, EstimatedTokensPerByte: 0.25, CapabilityRevision: "original-capability"}
	}
	if fixture().Validate() != nil {
		t.Fatal("valid owner capacity rejected")
	}
	for _, change := range []func(*PreparationProviderBudget){
		func(b *PreparationProviderBudget) { b.Version = "future" },
		func(b *PreparationProviderBudget) { b.Target.ProviderID = "other" },
		func(b *PreparationProviderBudget) { b.ContextWindowTokens = 0 },
		func(b *PreparationProviderBudget) { b.EffectiveMaxOutputTokens = b.ContextWindowTokens },
		func(b *PreparationProviderBudget) { b.ProviderMaxOutputTokens = 1 },
		func(b *PreparationProviderBudget) { b.MaxInputTokens = b.ContextWindowTokens },
		func(b *PreparationProviderBudget) { b.MaxRequestBytes = 0 },
		func(b *PreparationProviderBudget) { b.EstimatedTokensPerByte = math.NaN() },
		func(b *PreparationProviderBudget) { b.EstimatedTokensPerByte = math.Inf(1) },
		func(b *PreparationProviderBudget) { b.CapabilityRevision = "" },
		func(b *PreparationProviderBudget) { b.FailureCode = "capacity_unavailable" },
		func(b *PreparationProviderBudget) { b.FailureCode = "ready" },
	} {
		b := fixture()
		change(&b)
		if b.Validate() == nil {
			t.Fatal("invalid or mixed budget accepted")
		}
	}
	b := PreparationProviderBudget{Version: PreparationProviderBudgetVersion, Target: fixture().Target, FailureCode: "capacity_unavailable"}
	if b.Validate() != nil {
		t.Fatal("bounded failure rejected")
	}
}

func TestPreparationProviderBudgetResultRequiresExactFreshSource(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "exact"}
	command := ObserveProviderCatalogCacheCommand{WorkspaceID: "ws", Target: &target}
	base := catalogCacheResultFixture()
	base.Configuration = &PreparationProviderConfiguration{Version: PreparationProviderConfigurationVersion, DefaultChat: target}
	base.Budget = &PreparationProviderBudget{Version: PreparationProviderBudgetVersion, Target: target, ContextWindowTokens: 32000, ProviderMaxOutputTokens: 4000, EffectiveMaxOutputTokens: 4000, MaxInputTokens: 20000, MaxRequestBytes: 80000, EstimatedTokensPerByte: 0.25, CapabilityRevision: "original-capability"}
	if base.ValidateFor(command) != nil || base.ValidateFor(ObserveProviderCatalogCacheCommand{WorkspaceID: "ws"}) != nil {
		t.Fatal("exact explicit or default target rejected")
	}
	for name, mutate := range map[string]func(*ObserveProviderCatalogCacheResult){
		"missing_configuration": func(r *ObserveProviderCatalogCacheResult) { r.Configuration = nil },
		"foreign_target":        func(r *ObserveProviderCatalogCacheResult) { r.Budget.Target.ModelID = "foreign" },
		"malformed_capacity":    func(r *ObserveProviderCatalogCacheResult) { r.Budget.MaxInputTokens = 0 },
		"expired": func(r *ObserveProviderCatalogCacheResult) {
			r.ObservedAt = r.Catalog.ExpiresAt
			r.Catalog.State, r.Catalog.Stale, r.Catalog.ErrorCode = llmmodel.ProviderCatalogStateStale, true, "cache_expired"
		},
		"cache_miss": func(r *ObserveProviderCatalogCacheResult) {
			r.Catalog = llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodel.ProviderCatalogStateUnavailable, ErrorCode: "cache_miss"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := base.Clone()
			mutate(&r)
			if r.ValidateFor(command) == nil {
				t.Fatal("capacity accepted without exact fresh source")
			}
		})
	}
	foreignDefault := base.Clone()
	foreignDefault.Configuration.DefaultChat.ModelID = "foreign"
	if foreignDefault.ValidateFor(ObserveProviderCatalogCacheCommand{WorkspaceID: "ws"}) == nil || foreignDefault.ValidateFor(command) != nil {
		t.Fatal("default and explicit target intents were conflated")
	}
}
