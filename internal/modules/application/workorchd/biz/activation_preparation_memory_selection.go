package biz

import (
	"context"
	"reflect"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Observe actual active LLM-owner mode around the complete Memory inventory.
// Query failure never becomes an absent embedding target or a list-only pass.
// All failures still retain a complete inventory when one was obtained.
func (b *Biz) observePreparationMemoryFacts(ctx context.Context, original workspaceevents.CheckWorkspacePreparationCommand) (*workspace.PreparationMemoryInventoryFacts, error) {
	started := time.Now().UTC()
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: original.WorkspaceID, ExpectedWorkspaceRevision: original.ExpectedWorkspaceRevision + 1}
	exact, sealErr := b.QueryExactConfirmedStartupSnapshot(ctx, command)
	before, beforeErr := configcore.GetActiveLLMConfiguration(ctx, b.EventHub())
	beforeMode := (workspace.PreparationMemoryModeObservation{ProviderID: before.ProviderID, ConfigurationRevision: before.ConfigurationRevision, DefaultEmbedding: before.DefaultEmbedding}).Clone()
	inventory, err := b.ObservePreparationMemoryInventory(ctx, original.WorkspaceID)
	if err != nil {
		return nil, preparationStageError("inventory", err)
	}
	f := &workspace.PreparationMemoryInventoryFacts{Version: workspace.PreparationMemoryInventoryEvidenceVersion, WorkspaceRevision: command.ExpectedWorkspaceRevision, Observation: inventory.Clone()}
	if sealErr != nil {
		return f, preparationStageError("seal_before", sealErr)
	}
	if beforeErr != nil {
		return f, preparationStageError("configuration_before", beforeErr)
	}
	after, afterErr := configcore.GetActiveLLMConfiguration(ctx, b.EventHub())
	reobserved, reobserveErr := b.QueryExactConfirmedStartupSnapshot(ctx, command)
	if afterErr != nil {
		return f, preparationStageError("configuration_after", afterErr)
	}
	if reobserveErr != nil {
		return f, preparationStageError("seal_after", reobserveErr)
	}
	if err := ctx.Err(); err != nil {
		return f, preparationStageError("completed", err)
	}
	s := workspace.PreparationMemorySelectionSource{Version: workspace.PreparationMemorySelectionVersion, WorkspaceID: original.WorkspaceID, WorkspaceRevision: command.ExpectedWorkspaceRevision, SnapshotHash: exact.Snapshot.SnapshotHash, ReobservedSnapshotHash: reobserved.Snapshot.SnapshotHash, Before: beforeMode, After: workspace.PreparationMemoryModeObservation{ProviderID: after.ProviderID, ConfigurationRevision: after.ConfigurationRevision, DefaultEmbedding: after.DefaultEmbedding}, StartedAt: started, CompletedAt: time.Now().UTC()}
	if s.Before.Validate() == nil && s.After.Validate() == nil && !reflect.DeepEqual(s.Before, s.After) {
		return f, preparationStageError("configuration_drift", common.ErrConflict)
	}
	if s.SnapshotHash != s.ReobservedSnapshotHash {
		return f, preparationStageError("seal_drift", common.ErrConflict)
	}
	if err := s.Validate(); err != nil {
		return f, preparationStageError("validate_selection_source", err)
	} else {
		s = s.Clone()
		f.SelectionSource = &s
	}
	return f, nil
}
