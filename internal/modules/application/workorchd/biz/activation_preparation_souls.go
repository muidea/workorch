package biz

import (
	"context"
	"reflect"
	"slices"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
)

// PreparationSoulObservation contains bounded owner facts, not readiness or
// an execution permit. Unbound stages remain explicit; policy decides whether
// they are applicable. Soul allowlists are not proof of resource availability.
type PreparationSoulObservation struct {
	workspace.PreparationSoulFacts
	FailureCode        string
	Reobserved         bool
	SealBindingCurrent bool
}

func (b *Biz) ObservePreparationSouls(ctx context.Context, command workspaceevents.QueryExactConfirmedStartupSnapshotCommand) (PreparationSoulObservation, error) {
	value := PreparationSoulObservation{PreparationSoulFacts: workspace.PreparationSoulFacts{Version: "workorch-preparation-souls/v1", WorkspaceID: command.WorkspaceID, WorkspaceRevision: command.ExpectedWorkspaceRevision, StartedAt: time.Now().UTC()}}
	fail := func(code string, err error) (PreparationSoulObservation, error) {
		value.FailureCode = code
		value.CompletedAt = time.Now().UTC()
		return value, err
	}
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return fail("invalid_request", common.ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return fail("cancelled", err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return fail("scope_conflict", err)
	}
	bounded, cancel := context.WithTimeout(bound, 3*time.Second)
	defer cancel()
	// A missing loader is not an observed empty system default. Never fall back
	// to process-global configuration or call the full activation builder.
	if configcore.LoaderFromContext(bounded) == nil {
		return fail("configuration_unavailable", common.ErrConflict)
	}
	cfg, err := configcore.LoadFromContext(bounded)
	if err != nil {
		return fail("configuration_unavailable", err)
	}
	value.SystemDefaultSoulID = common.SoulID(cfg.Runtime.DefaultSoulID)
	if value.SystemDefaultSoulID != "" && !preparationSoulIDValid(value.SystemDefaultSoulID) {
		return fail("invalid_system_soul", common.ErrConflict)
	}
	before, err := b.QueryExactConfirmedStartupSnapshot(bounded, command)
	if err != nil {
		return fail("seal_unavailable", err)
	}
	value.SnapshotHash = before.Snapshot.SnapshotHash
	value.ConfigurationHash = before.Snapshot.AuthoringConfigurationHash
	w, err := ownerreadports.ReadWorkspaceByOwnerCommand(bounded, b.EventHub(), command.WorkspaceID)
	if err != nil {
		return fail("configuration_unavailable", err)
	}
	if w == nil || w.ID != command.WorkspaceID || w.Revision != command.ExpectedWorkspaceRevision || workspace.WorkspaceConfigurationHash(w) != value.ConfigurationHash {
		return fail("configuration_drift", common.ErrConflict)
	}
	value.WorkspaceDefaultSoulID = w.DefaultSoulID
	value.WorkspaceBindings = soulbinding.FromSoulIDMap(w.SoulBindings)
	plan, defaultLookup, err := preparationSoulPlan(w, value.SystemDefaultSoulID)
	if err != nil {
		return fail("invalid_workspace_soul_bindings", err)
	}
	value.Selections, value.DefaultLookupSoulID = plan, defaultLookup
	ids := []common.SoulID{}
	add := func(id common.SoulID) {
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	add(defaultLookup)
	for _, selection := range plan {
		add(selection.SoulID)
	}
	for _, id := range ids {
		item, err := ownerreadports.ReadSoulByOwnerCommand(bounded, b.EventHub(), id)
		if err != nil {
			return fail("soul_unavailable", err)
		}
		if !validPreparationSoul(item, id) {
			return fail("invalid_soul_snapshot", common.ErrConflict)
		}
		value.Souls = append(value.Souls, clonePreparationSoul(item))
	}
	// Re-read the entire selected records, not just hashes. This detects
	// revision/provenance/list movement as well as content replacement.
	for i, id := range ids {
		current, err := ownerreadports.ReadSoulByOwnerCommand(bounded, b.EventHub(), id)
		if err != nil {
			return fail("soul_reobservation_failed", err)
		}
		value.ReobservedSouls = append(value.ReobservedSouls, clonePreparationSoul(current))
		if !validPreparationSoul(current, id) || !reflect.DeepEqual(value.Souls[i], current) {
			return fail("soul_drift", common.ErrConflict)
		}
	}
	afterCfg, err := configcore.LoadFromContext(bounded)
	if err != nil {
		return fail("configuration_unavailable", err)
	}
	value.ReobservedSystemDefaultSoulID = common.SoulID(afterCfg.Runtime.DefaultSoulID)
	if afterCfg.Runtime.DefaultSoulID != string(value.SystemDefaultSoulID) {
		return fail("system_soul_drift", common.ErrConflict)
	}
	after, err := b.QueryExactConfirmedStartupSnapshot(bounded, command)
	if err != nil {
		return fail("seal_reobservation_failed", err)
	}
	value.ReobservedSnapshotHash = after.Snapshot.SnapshotHash
	if after.Snapshot.SnapshotHash != value.SnapshotHash {
		return fail("seal_drift", common.ErrConflict)
	}
	if err := bounded.Err(); err != nil {
		return fail("cancelled", err)
	}
	value.Reobserved, value.SealBindingCurrent = true, true
	value.CompletedAt = time.Now().UTC()
	return value, nil
}

func preparationSoulPlan(w *workspace.Workspace, systemDefault common.SoulID) ([]workspace.PreparationSoulSelection, common.SoulID, error) {
	if w == nil {
		return nil, "", common.ErrConflict
	}
	return workspace.PreparationSoulPlan(w.DefaultSoulID, soulbinding.FromSoulIDMap(w.SoulBindings), systemDefault)
}

func preparationSoulIDValid(id common.SoulID) bool {
	return soulmodel.ValidPreparationSoulID(id)
}

func validPreparationSoul(item *soulmodel.Soul, id common.SoulID) bool {
	return soulmodel.ValidPreparationSnapshot(item, id)
}

func clonePreparationSoul(item *soulmodel.Soul) *soulmodel.Soul {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.BusinessDomains = slices.Clone(item.BusinessDomains)
	cloned.AllowedSkillRefs = slices.Clone(item.AllowedSkillRefs)
	cloned.AllowedMCPServerRefs = slices.Clone(item.AllowedMCPServerRefs)
	cloned.AllowedCapabilityRefs = slices.Clone(item.AllowedCapabilityRefs)
	cloned.AllowedCapabilityCategories = slices.Clone(item.AllowedCapabilityCategories)
	cloned.AllowedToolKinds = slices.Clone(item.AllowedToolKinds)
	return &cloned
}
