package biz

import (
	"context"
	"slices"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/skillselectcore"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/skillselection"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// PreparationSkillSelectionObservation is restricted source/selection material,
// not a PASS, M3, atomic cross-owner snapshot or sealed Runtime context. All
// original status and instruction bytes remain available on partial failure.
// Status/currentness, auxiliary resources and context budgets need their own
// checks; a compiled selection never grants availability or execution rights.
type PreparationSkillSelectionObservation struct {
	Version            string
	WorkspaceID        common.WorkspaceID
	WorkspaceRevision  uint64
	SnapshotHash       string
	GoalText           string
	DoDItems           []string
	SoulObservation    PreparationSoulObservation
	ExecutionSoulID    common.SoulID
	AllowedSkillRefs   []string
	Registry           PreparationSkillRegistryObservation
	Global             contextregistryports.SkillHubPreparationCatalog
	Workspace          contextregistryports.SkillHubPreparationCatalog
	Selection          skillselection.TypedSelection
	SelectedSkills     []workorchmanagementcontract.Skill
	StartedAt          time.Time
	CompletedAt        time.Time
	FailureCode        string
	SelectionCompiled  bool
	SealBindingCurrent bool
}

func (b *Biz) ObservePreparationSkillSelection(ctx context.Context, command workspaceevents.QueryExactConfirmedStartupSnapshotCommand) (PreparationSkillSelectionObservation, error) {
	return b.observePreparationSkillSelection(ctx, command, contextregistryports.SkillHubPreparationExecRunner{})
}

func (b *Biz) observePreparationSkillSelection(ctx context.Context, command workspaceevents.QueryExactConfirmedStartupSnapshotCommand, runner contextregistryports.SkillHubPreparationRunner) (PreparationSkillSelectionObservation, error) {
	value := PreparationSkillSelectionObservation{Version: "workorch-preparation-skill-selection/v1", WorkspaceID: command.WorkspaceID, WorkspaceRevision: command.ExpectedWorkspaceRevision, StartedAt: time.Now().UTC()}
	fail := func(code string, err error) (PreparationSkillSelectionObservation, error) {
		value.FailureCode, value.CompletedAt = code, time.Now().UTC()
		return value, err
	}
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil || runner == nil {
		return fail("invalid_request", common.ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return fail("cancelled", err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return fail("scope_conflict", err)
	}
	bounded, cancel := context.WithTimeout(bound, 5*time.Second)
	defer cancel()
	before, err := b.QueryExactConfirmedStartupSnapshot(bounded, command)
	if err != nil {
		return fail("seal_unavailable", err)
	}
	value.SnapshotHash, value.GoalText, value.DoDItems = before.Snapshot.SnapshotHash, before.Snapshot.Goal.Text, slices.Clone(before.Snapshot.DoD.Items)
	value.SoulObservation, err = b.ObservePreparationSouls(bounded, command)
	if err != nil {
		return fail("soul_observation_failed", err)
	}
	if value.SoulObservation.SnapshotHash != value.SnapshotHash {
		return fail("seal_drift", common.ErrConflict)
	}
	for _, selection := range value.SoulObservation.Selections {
		if selection.Stage == "execution" {
			value.ExecutionSoulID = selection.SoulID
		}
	}
	if value.ExecutionSoulID != "" {
		found := false
		for _, soul := range value.SoulObservation.Souls {
			if soul.ID == value.ExecutionSoulID {
				value.AllowedSkillRefs, found = slices.Clone(soul.AllowedSkillRefs), true
			}
		}
		if !found {
			return fail("execution_soul_missing", common.ErrConflict)
		}
	}
	value.Registry, err = b.ObservePreparationSkillRegistry(bounded, command)
	if err != nil {
		return fail("registry_observation_failed", err)
	}
	if value.Registry.SnapshotHash != value.SnapshotHash {
		return fail("seal_drift", common.ErrConflict)
	}
	value.Global, err = contextregistryports.ObserveSkillHubPreparationCatalog(bounded, runner, command.WorkspaceID, "", true)
	if err != nil {
		return fail("global_skillhub_observation_failed", err)
	}
	value.Workspace, err = contextregistryports.ObserveSkillHubPreparationCatalog(bounded, runner, command.WorkspaceID, before.Snapshot.StartupInput.ResolvedProjectWorkspaceDirectory, false)
	if err != nil {
		return fail("workspace_skillhub_observation_failed", err)
	}
	catalog := make([]workorchmanagementcontract.Skill, 0, len(value.Registry.Catalog.Skills)+len(value.Global.Skills)+len(value.Workspace.Skills))
	for _, item := range value.Registry.Catalog.Skills {
		catalog = append(catalog, contextregistrycore.MapRepositorySkill(item))
	}
	catalog = append(catalog, value.Global.Skills...)
	catalog = append(catalog, value.Workspace.Skills...)
	value.Selection, err = skillselection.AutoSelectTyped(skillselectcore.CandidatesFromSkills(catalog), value.GoalText, value.DoDItems, value.AllowedSkillRefs)
	if err != nil {
		return fail("selection_conflict", err)
	}
	// AutoSelectTyped checked full-catalog uniqueness. Materialize every match
	// in exact selection order; missing rows are errors, never silently skipped.
	byID := make(map[string]workorchmanagementcontract.Skill, len(catalog))
	for _, item := range catalog {
		byID[item.ID] = item
	}
	for _, match := range value.Selection.Matches {
		item, present := byID[match.Candidate.ID]
		if !present {
			return fail("selection_material_missing", common.ErrConflict)
		}
		value.SelectedSkills = append(value.SelectedSkills, clonePreparationSelectedSkill(item))
	}
	value.SelectionCompiled = true
	after, err := b.QueryExactConfirmedStartupSnapshot(bounded, command)
	if err != nil {
		return fail("seal_reobservation_failed", err)
	}
	if after.Snapshot.SnapshotHash != value.SnapshotHash {
		return fail("seal_drift", common.ErrConflict)
	}
	if err := bounded.Err(); err != nil {
		return fail("cancelled", err)
	}
	value.SealBindingCurrent, value.CompletedAt = true, time.Now().UTC()
	return value, nil
}

func clonePreparationSelectedSkill(item workorchmanagementcontract.Skill) workorchmanagementcontract.Skill {
	item.TriggerRules = slices.Clone(item.TriggerRules)
	item.ReferenceRefs = slices.Clone(item.ReferenceRefs)
	item.RequiredToolKinds = slices.Clone(item.RequiredToolKinds)
	item.RequiredMCPServerRefs = slices.Clone(item.RequiredMCPServerRefs)
	item.RequiredCapabilityRefs = slices.Clone(item.RequiredCapabilityRefs)
	item.Constraints = slices.Clone(item.Constraints)
	return item
}
