package llm

import (
	"context"
	"slices"
	"sort"
	"time"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// observeCache is deliberately separate from GetCatalog: no HTTP, fallback,
// eviction, inflight wait, retry, or map initialization is allowed on this path.
// The credential-dependent cache key remains private to the LLM owner.
func (s *CatalogService) observeCache(ctx context.Context, cfg workorchconfig.LLMConfig) (llmmodels.ProviderModelCatalog, time.Time, error) {
	if s == nil || ctx == nil || s.now == nil {
		return llmmodels.ProviderModelCatalog{}, time.Time{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return llmmodels.ProviderModelCatalog{}, time.Time{}, err
	}
	cfg, err := workorchconfig.NormalizeLLMConfig(cfg)
	if err != nil {
		// Never propagate a configuration value into a cross-owner diagnostic.
		return llmmodels.ProviderModelCatalog{}, time.Time{}, common.ErrConflict
	}
	key := catalogCacheKeyFromConfig(cfg)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return llmmodels.ProviderModelCatalog{}, time.Time{}, err
	}
	now := s.now().UTC()
	entry := s.entries[key]
	if entry == nil {
		return llmmodels.ProviderModelCatalog{ProviderID: runtimecontract.BuiltinAIProxyProviderID, Source: "ai_proxy", State: llmmodels.ProviderCatalogStateUnavailable, ErrorCode: "cache_miss"}, now, nil
	}
	if len(entry.catalog.Models) > llmevents.CatalogCacheObservationModelLimit || entry.key != key || entry.connectionRev != key.ConnectionRevision {
		return llmmodels.ProviderModelCatalog{}, time.Time{}, common.ErrConflict
	}
	// Validate under the owner lock, then copy without normalization or changing
	// the stored declaration. Do not detach a corrupt unbounded subtree first.
	catalog := entry.catalog
	// Successful cached snapshots have no read-observation/diagnostic fields.
	// Reject unexpected stored diagnostics rather than erase them and promote
	// the remaining declaration into an apparently successful observation.
	if catalog.State != "" || catalog.Stale || catalog.ErrorMessage != "" || catalog.ErrorCode != "" {
		return llmmodels.ProviderModelCatalog{}, time.Time{}, common.ErrConflict
	}
	catalog.Stale = !now.Before(catalog.ExpiresAt)
	catalog.State = llmmodels.ProviderCatalogStateFresh
	if catalog.Stale {
		catalog.State, catalog.ErrorCode = llmmodels.ProviderCatalogStateStale, "cache_expired"
	}
	// Check the complete bounded shape before re-encoding its revision. A
	// corrupt cache is not permission to copy/hash an unbounded declaration.
	if llmevents.ValidateProviderCatalogCacheSnapshot(catalog, now) != nil || catalog.Revision != catalogRevision(key.ProviderID, catalog.Models) {
		return llmmodels.ProviderModelCatalog{}, time.Time{}, common.ErrConflict
	}
	return (llmevents.ObserveProviderCatalogCacheResult{Catalog: catalog}).Clone().Catalog, now, nil
}

func (m *Biz) observeProviderCatalogCache(ctx context.Context, command llmevents.ObserveProviderCatalogCacheCommand) (llmevents.ObserveProviderCatalogCacheResult, error) {
	if m == nil || command.Validate() != nil || m.preparationConfigurationEpoch == "" {
		return llmevents.ObserveProviderCatalogCacheResult{}, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return llmevents.ObserveProviderCatalogCacheResult{}, err
	}
	cfg, err := m.activeConfiguration()
	if err != nil {
		return llmevents.ObserveProviderCatalogCacheResult{}, common.ErrConflict
	}
	catalog, now, err := m.catalogService.observeCache(bound, cfg)
	if err != nil {
		return llmevents.ObserveProviderCatalogCacheResult{}, err
	}
	value := llmevents.ObserveProviderCatalogCacheResult{WorkspaceID: command.WorkspaceID, OwnerEpoch: m.preparationConfigurationEpoch, ObservedAt: now, Catalog: catalog}
	if len(cfg.AIProxy.ModelOverrides) > llmevents.CatalogCacheObservationModelLimit {
		return llmevents.ObserveProviderCatalogCacheResult{}, common.ErrConflict
	}
	configuration := llmevents.PreparationProviderConfiguration{Version: llmevents.PreparationProviderConfigurationSnapshotVersion, DefaultChat: cfg.DefaultChat.ToRuntimeTarget()}
	for modelID, override := range cfg.AIProxy.ModelOverrides {
		if len(override.Operations) > 64 {
			return llmevents.ObserveProviderCatalogCacheResult{}, common.ErrConflict
		}
		configuration.Overrides = append(configuration.Overrides, llmevents.PreparationProviderModelOverride{ModelID: modelID, ContextWindowTokens: override.ContextWindowTokens, MaxOutputTokens: override.MaxOutputTokens, Operations: slices.Clone(override.Operations)})
	}
	sort.Slice(configuration.Overrides, func(i, j int) bool { return configuration.Overrides[i].ModelID < configuration.Overrides[j].ModelID })
	value.Configuration = &configuration
	if catalog.State == llmmodels.ProviderCatalogStateFresh {
		target := configuration.DefaultChat
		if command.Target != nil {
			target = *command.Target
		}
		budget := llmevents.PreparationProviderBudget{Version: llmevents.PreparationProviderBudgetVersion, Target: target, FailureCode: "capacity_unavailable"}
		// Reuse the exact production resolver, with the already observed cache.
		// Do not call resolveTarget/getCatalog or export its credential-derived
		// ConfigurationRevision. No client or request allocation is constructed.
		profile, resolveErr := ResolveProviderModelTarget(cfg, &catalog, target, workorchconfig.LLMOperationResponses)
		if resolveErr == nil {
			budget = llmevents.PreparationProviderBudget{Version: llmevents.PreparationProviderBudgetVersion, Target: profile.Target, ContextWindowTokens: profile.ContextWindowTokens, ProviderMaxOutputTokens: profile.ProviderMaxOutputTokens, EffectiveMaxOutputTokens: profile.EffectiveMaxOutputTokens, MaxInputTokens: profile.MaxInputTokens, MaxRequestBytes: profile.MaxRequestBytes, EstimatedTokensPerByte: profile.EstimatedTokensPerByte, CapabilityRevision: profile.CapabilityRevision}
		}
		value.Budget = &budget
	}
	if err := value.ValidateFor(command); err != nil {
		return llmevents.ObserveProviderCatalogCacheResult{}, err
	}
	return value, nil
}
