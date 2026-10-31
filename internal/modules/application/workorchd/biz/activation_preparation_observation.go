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
	phases := newPreparationObservationPhases(ctx)
	defer phases.stopInitial()
	bounded := phases.initial
	settlement := workspaceevents.SettleWorkspacePreparationBlockedCommand{Original: original, ProviderCacheReason: "provider_cache_unavailable", RunHistoryReason: "run_history_unavailable"}
	observe := func(owner string, f func() (bool, error)) {
		start := time.Now().UTC()
		attempted := bounded.Err() == nil
		complete, err := false, bounded.Err()
		if attempted {
			complete, err = f()
		}
		settlement.Observations = append(settlement.Observations, preparationDiagnostic(bounded, "initial", owner, "observe", start, attempted, complete, err))
	}
	var target *runtimecontract.ProviderModelTarget
	if providerDefinition != nil && providerDefinition.WorkspaceDefault != nil {
		v := providerDefinition.WorkspaceDefault.Normalized().Target
		// Invalid declarations still take the original diagnostic path. They
		// cannot obtain budget credit from the owner's default or another model.
		if v.Validate() == nil && v.ProviderID == runtimecontract.BuiltinAIProxyProviderID {
			target = &v
		}
	}
	observe("llm", func() (bool, error) {
		catalog, err := b.observePreparationProviderCatalogFor(bounded, original.WorkspaceID, target)
		if err != nil {
			return false, err
		}
		facts := workspace.PreparationProviderCacheFacts{Version: workspace.PreparationProviderCacheEvidenceVersion, WorkspaceRevision: original.ExpectedWorkspaceRevision + 1, Observation: catalog.Clone()}
		settlement.ProviderCacheFacts, settlement.ProviderCacheReason = &facts, facts.BlockedReason()
		return true, nil
	})
	observe("run", func() (bool, error) {
		history, err := b.ObservePreparationRunHistory(bounded, original.WorkspaceID, original.RequestID)
		if err != nil {
			return false, err
		}
		facts := workspace.PreparationRunHistoryFacts{Version: workspace.PreparationRunHistoryEvidenceVersion, WorkspaceRevision: original.ExpectedWorkspaceRevision + 1, Observation: history.Clone()}
		if err := facts.Validate(); err != nil {
			return false, preparationStageError("validate", err)
		}
		settlement.RunHistoryFacts, settlement.RunHistoryReason = &facts, facts.BlockedReason()
		return true, nil
	})
	settlement.SoulReason = "soul_observation_unavailable_not_saved"
	settlement.MemoryInventoryReason = "memory_inventory_unavailable_not_saved"
	settlement.SkillRegistryReason = "skill_registry_observation_unavailable_not_saved"
	settlement.MCPRegistryReason = "mcp_registry_observation_unavailable_not_saved"
	observe("memory", func() (bool, error) {
		facts, err := b.observePreparationMemoryFacts(bounded, original)
		if facts != nil {
			settlement.MemoryInventoryFacts, settlement.MemoryInventoryReason = facts, facts.BlockedReason()
		}
		return facts != nil && facts.SelectionSource != nil, err
	})
	observe("mcpregistry", func() (bool, error) {
		facts, err := b.observePreparationMCPRegistry(bounded, original)
		if err == nil {
			settlement.MCPRegistryFacts, settlement.MCPRegistryReason = facts, "mcp_registry_observed_connection_pending"
		}
		return facts != nil, err
	})
	c := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: original.WorkspaceID, ExpectedWorkspaceRevision: original.ExpectedWorkspaceRevision + 1}
	observe("soul", func() (bool, error) {
		value, err := b.ObservePreparationSouls(bounded, c)
		if err != nil {
			return false, preparationStageError(value.FailureCode, err)
		}
		if err = value.PreparationSoulFacts.Validate(); err != nil {
			return false, preparationStageError("validate", err)
		}
		facts := value.PreparationSoulFacts.Clone()
		settlement.SoulFacts, settlement.SoulReason = &facts, "souls_observed_remaining_checks"
		return true, nil
	})
	observe("skill", func() (bool, error) {
		value, err := b.ObservePreparationSkillRegistry(bounded, c)
		if err != nil {
			return false, preparationStageError(value.FailureCode, err)
		}
		facts := value.Facts()
		if err = facts.Validate(); err != nil {
			return false, preparationStageError("validate", err)
		}
		settlement.SkillRegistryFacts, settlement.SkillRegistryReason = &facts, "skill_registry_observed_selection_pending"
		return true, nil
	})
	if plan != nil {
		settlement.SkillHubEffectRef = plan.Ref
		// The adapter records reserved-but-not-started or unknown effects as
		// such; cancellation must never authorize a replacement invocation.
		settlement.SkillHubGlobal, settlement.SkillHubProject = b.observeReservedPreparationSkillHub(bounded, *plan)
	}
	observe("process", func() (bool, error) {
		facts, err := observePreparationProcessPolicy(bounded, original)
		settlement.ProcessPolicyFacts = facts
		return facts != nil, err
	})
	// Missing original sources already block this attempt. Do not repeat known
	// failed reads or collect a second unrelated inventory just to fill a shape.
	if settlement.SoulFacts != nil && settlement.SkillRegistryFacts != nil && settlement.MemoryInventoryFacts != nil && settlement.MemoryInventoryFacts.SelectionSource != nil && settlement.MCPRegistryFacts != nil {
		finalCtx, finish := phases.beginFinal()
		var diagnostics []workspace.PreparationObservationDiagnostic
		settlement.FinalOwners, diagnostics = b.observePreparationFinalOwners(finalCtx, original)
		settlement.Observations = append(settlement.Observations, diagnostics...)
		finish()
	}
	// Use the original request context, not a detached background context. If
	// cancelled before settlement, the durable accepted cut remains recoverable;
	// explicit recovery closes interrupted work without redoing either query.
	if err := ctx.Err(); err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	settleCtx, stop := context.WithTimeout(ctx, preparationSettlementBudget)
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
