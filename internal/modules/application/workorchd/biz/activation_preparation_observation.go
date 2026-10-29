package biz

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only first acceptance calls owner observations and the reserved SkillHub
// status adapter. No catalog refresh, inference, embedding or execution intent.
// Cache/history retain complete original facts, not readiness summaries.
// Complete Soul/Skill registry before/after values use the historical archive contract;
// neither path can grant preparation credit.
func (b *Biz) observeAndSettleBlockedPreparation(ctx context.Context, original workspaceevents.CheckWorkspacePreparationCommand, plan *workspace.WorkspacePreparationSkillHubEffects, providerDefinition *workspace.PreparationProviderSelectionDefinition) (workspaceevents.CheckWorkspacePreparationResult, error) {
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	settlement := workspaceevents.SettleWorkspacePreparationBlockedCommand{Original: original, ProviderCacheReason: "provider_cache_unavailable", RunHistoryReason: "run_history_unavailable"}
	var target *runtimecontract.ProviderModelTarget
	if providerDefinition != nil && providerDefinition.WorkspaceDefault != nil {
		v := providerDefinition.WorkspaceDefault.Normalized().Target
		// Invalid declarations still take the original diagnostic path. They
		// cannot obtain budget credit from the owner's default or another model.
		if v.Validate() == nil && v.ProviderID == runtimecontract.BuiltinAIProxyProviderID {
			target = &v
		}
	}
	if catalog, err := b.observePreparationProviderCatalogFor(bounded, original.WorkspaceID, target); err == nil {
		facts := workspace.PreparationProviderCacheFacts{Version: workspace.PreparationProviderCacheEvidenceVersion, WorkspaceRevision: original.ExpectedWorkspaceRevision + 1, Observation: catalog.Clone()}
		settlement.ProviderCacheFacts = &facts
		settlement.ProviderCacheReason = facts.BlockedReason()
	}
	if history, err := b.ObservePreparationRunHistory(bounded, original.WorkspaceID, original.RequestID); err == nil {
		facts := workspace.PreparationRunHistoryFacts{Version: workspace.PreparationRunHistoryEvidenceVersion, WorkspaceRevision: original.ExpectedWorkspaceRevision + 1, Observation: history.Clone()}
		if facts.Validate() == nil {
			settlement.RunHistoryFacts, settlement.RunHistoryReason = &facts, facts.BlockedReason()
		}
	}
	settlement.SoulReason = "soul_observation_unavailable_not_saved"
	settlement.MemoryInventoryReason = "memory_inventory_unavailable_not_saved"
	if original.ExpectedWorkspaceRevision != ^uint64(0) {
		if facts, err := b.observePreparationMemoryFacts(bounded, original); err == nil {
			settlement.MemoryInventoryFacts, settlement.MemoryInventoryReason = facts, facts.BlockedReason()
		}
	}
	settlement.SkillRegistryReason = "skill_registry_observation_unavailable_not_saved"
	settlement.MCPRegistryReason = "mcp_registry_observation_unavailable_not_saved"
	if original.ExpectedWorkspaceRevision != ^uint64(0) {
		if facts, err := b.observePreparationMCPRegistry(bounded, original); err == nil {
			settlement.MCPRegistryFacts, settlement.MCPRegistryReason = facts, "mcp_registry_observed_connection_pending"
		}
	}
	if original.ExpectedWorkspaceRevision != ^uint64(0) {
		observed, soulErr := b.ObservePreparationSouls(bounded, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: original.WorkspaceID, ExpectedWorkspaceRevision: original.ExpectedWorkspaceRevision + 1})
		// Compatibility helper flags are deliberately not sent or trusted.
		if soulErr == nil && observed.PreparationSoulFacts.Validate() == nil {
			facts := observed.PreparationSoulFacts.Clone()
			settlement.SoulFacts = &facts
			settlement.SoulReason = "souls_observed_remaining_checks"
		}
		registry, registryErr := b.ObservePreparationSkillRegistry(bounded, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: original.WorkspaceID, ExpectedWorkspaceRevision: original.ExpectedWorkspaceRevision + 1})
		if facts := registry.Facts(); registryErr == nil && facts.Validate() == nil {
			settlement.SkillRegistryFacts = &facts
			settlement.SkillRegistryReason = "skill_registry_observed_selection_pending"
		}
	}
	if plan != nil {
		settlement.SkillHubEffectRef = plan.Ref
		settlement.SkillHubGlobal, settlement.SkillHubProject = b.observeReservedPreparationSkillHub(bounded, *plan)
	}
	if facts, err := observePreparationProcessPolicy(bounded, original); err == nil {
		settlement.ProcessPolicyFacts = facts
	}
	// Missing original sources already block this attempt. Do not repeat known
	// failed reads or collect a second unrelated inventory just to fill a shape.
	if settlement.SoulFacts != nil && settlement.SkillRegistryFacts != nil && settlement.MemoryInventoryFacts != nil && settlement.MemoryInventoryFacts.SelectionSource != nil && settlement.MCPRegistryFacts != nil {
		settlement.FinalOwners = b.observePreparationFinalOwners(bounded, original)
	}
	// Use the original request context, not a detached background context. If
	// cancelled before settlement, the durable accepted cut remains recoverable;
	// explicit recovery closes interrupted work without redoing either query.
	if err := ctx.Err(); err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	settleCtx, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspacePreparationSettleBlocked, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), settleCtx, settlement)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(original.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	value, err := result.Get()
	response, ok := value.(workspaceevents.CheckWorkspacePreparationResult)
	if err != nil || !ok || response.ObservationRequired || response.ValidateFor(original) != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, common.ErrConflict
	}
	return response, nil
}
