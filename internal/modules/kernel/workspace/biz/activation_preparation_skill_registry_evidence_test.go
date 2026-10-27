package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	skillowner "github.com/muidea/workorch/internal/modules/blocks/skill/biz"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

type preparationSkillArchiveRepository struct {
	repository.SkillRepository
	reads    int
	scenario string
}

func (r *preparationSkillArchiveRepository) List(ctx context.Context) ([]*skillmodel.Skill, error) {
	r.reads++
	if r.scenario == "error" && r.reads == 2 {
		return nil, errors.New("original registry read failure")
	}
	items, err := r.SkillRepository.List(ctx)
	if r.scenario == "drift" && r.reads == 2 && len(items) > 0 {
		items[0].Instruction = "changed instruction"
		items[0].Provenance.ContentHash = workorchskillcontract.StableContentHash(items[0].Content())
	}
	return items, err
}

func TestPreparationSkillRegistryEvidenceOriginalSettlementRecoveryAndReopen(t *testing.T) {
	for _, scenario := range []string{"complete", "empty", "missing_owner", "drift", "error", "write_failed", "reply_lost", "revision_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			storage := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, storage)
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-registry-archive")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "skills.json")
			repo := filerepository.NewSkillRepository(path)
			if scenario != "empty" {
				if err := repo.Create(ctx, preparationSkillFixture()); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(path)
			observed := &preparationSkillArchiveRepository{SkillRepository: repo, scenario: scenario}
			if scenario != "missing_owner" {
				owner, err := skillowner.New(owners.hub, nil, skillowner.Dependencies{Skills: observed})
				if err != nil {
					t.Fatal(err)
				}
				if err := owner.Start(); err != nil {
					t.Fatal(err)
				}
				defer owner.Teardown(ctx)
			}
			var calls atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, "miss", &calls, nil)
			if scenario == "revision_drift" {
				owners.workspace.workspaces = &preparationDirectoryDriftRepository{WorkspaceRepository: owners.workspace.workspaces}
			}
			if scenario == "write_failed" || scenario == "reply_lost" {
				owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 2, persistFirst: scenario == "reply_lost"}
			}
			app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			op, err := app.CheckStartupPreparation(ctx, w.ID, request, false)
			if (err != nil) != (scenario == "write_failed" || scenario == "reply_lost") {
				t.Fatal("unexpected settlement", err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
				t.Fatal("invalid archive", err)
			}
			a := stored.StartupPreparation.Attempts[0]
			complete := scenario == "complete" || scenario == "empty" || scenario == "reply_lost" || scenario == "revision_drift"
			if complete {
				if len(stored.PreparationSkillRegistryEvidence) != 1 || a.SkillRegistryEvidenceRef == "" {
					t.Fatal("missing full registry archive")
				}
				e := stored.PreparationSkillRegistryEvidence[0]
				if e.Ref != a.SkillRegistryEvidenceRef || e.RequestID != request.RequestID || e.Facts.WorkspaceRevision != w.Revision+1 || !reflect.DeepEqual(e.Facts.Catalog.Skills, e.Facts.ReobservedCatalog.Skills) {
					t.Fatal("lost original binding")
				}
				if scenario == "empty" {
					if e.Facts.Catalog.Skills == nil || len(e.Facts.Catalog.Skills) != 0 {
						t.Fatal("empty owner catalog not preserved")
					}
				} else if !reflect.DeepEqual(e.Facts.Catalog.Skills[0], preparationSkillFixture()) {
					t.Fatal("original record changed")
				}
				public, _ := json.Marshal(op)
				ordinary, _ := json.Marshal(stored)
				for _, secret := range []string{"原始指令", e.ContentHash, "PreparationSkillRegistryEvidence", "ReobservedCatalog"} {
					if bytes.Contains(public, []byte(secret)) || bytes.Contains(ordinary, []byte(secret)) {
						t.Fatal("private catalog leaked", secret)
					}
				}
			} else if len(stored.PreparationSkillRegistryEvidence) != 0 || a.SkillRegistryEvidenceRef != "" {
				t.Fatal("partial observation marked complete")
			}
			if scenario != "write_failed" {
				want := "skill_registry_observation_unavailable_not_saved"
				if complete {
					want = "skill_selection_sources_incomplete"
					if scenario != "revision_drift" && (a.SkillSelectionState != "sources_incomplete" || stored.PreparationSkillRegistryEvidence[0].Selection == nil || len(stored.PreparationSkillRegistryEvidence[0].Selection.Selected) != 0) {
						t.Fatal("missing SkillHub/Soul source turned into selection proof")
					}
				}
				if scenario == "revision_drift" {
					want = "binding_changed"
				}
				found := false
				for _, c := range a.Checks {
					if string(c.State) != expectedPreparationCheckState(c.ID, c.Reason) {
						t.Fatal("incorrect per-check judgment", c)
					}
					if c.ID == "skill_selection" {
						found = true
						if c.Reason != want {
							t.Fatal("wrong classification", c.Reason, want)
						}
					}
				}
				if !found {
					t.Fatal("skill check omitted")
				}
			}
			reads := observed.reads
			for _, recoverOriginal := range []bool{false, true, false} {
				if _, err := app.CheckStartupPreparation(ctx, w.ID, request, recoverOriginal); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := app.ReadStartupPreparationView(ctx, w.ID); err != nil {
				t.Fatal(err)
			}
			reopened := openFileDefinitionAdmissionOwners(t, storage)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(stored.PreparationSkillRegistryEvidence, read.PreparationSkillRegistryEvidence) || reads != observed.reads || calls.Load() != 2 {
				t.Fatal("replay/recovery reobserved or changed archive", err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("check wrote Skill registry")
			}
			if scenario == "complete" {
				for _, change := range []string{"erase", "ref", "rehash"} {
					bad, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
					switch change {
					case "erase":
						bad.PreparationSkillRegistryEvidence = nil
					case "ref":
						bad.StartupPreparation.Attempts[0].SkillRegistryEvidenceRef = ""
					case "rehash":
						e := &bad.PreparationSkillRegistryEvidence[0]
						for _, c := range []*skillmodel.PreparationRegistryCatalog{&e.Facts.Catalog, &e.Facts.ReobservedCatalog} {
							c.Skills[0].Instruction = "replacement"
							c.Skills[0].Provenance.ContentHash = workorchskillcontract.StableContentHash(c.Skills[0].Content())
							c.CatalogHash, _ = skillmodel.RegistryCatalogHash(c.Skills)
						}
						e.ContentHash = e.Hash()
					}
					if err := owners.repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
						t.Fatal("rewrote completed history", change)
					}
				}
			}
			runs, _ := owners.repositories.Runs.List(ctx)
			if len(runs) != 0 || read.ActiveRunID != "" || !reflect.DeepEqual(read.PreparationFence, w.PreparationFence) {
				t.Fatal("check changed execution authority")
			}
		})
	}
}

func TestPreparationSkillRegistryEvidenceRebindsOriginalOwnerInputs(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-registry-bind")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	original := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original", ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, original, false, true); err != nil {
		t.Fatal(err)
	}
	repo := filerepository.NewSkillRepository(filepath.Join(t.TempDir(), "skills.json"))
	if err := repo.Create(ctx, preparationSkillFixture()); err != nil {
		t.Fatal(err)
	}
	owner, err := skillowner.New(owners.hub, nil, skillowner.Dependencies{Skills: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)
	app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	observed, err := app.ObservePreparationSkillRegistry(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision + 1})
	if err != nil {
		t.Fatal(err)
	}
	facts := observed.Facts()
	c := workspaceevents.SettleWorkspacePreparationBlockedCommand{Original: original, ProviderCacheReason: "provider_cache_miss", RunHistoryReason: "run_history_empty_fence_not_final", SkillRegistryReason: "skill_registry_observed_selection_pending", SkillRegistryFacts: &facts}
	if c.Validate() != nil {
		t.Fatal("invalid valid command")
	}
	if e, _ := owners.workspace.bindOriginalPreparationSkillRegistry(ctx, c); e == nil {
		t.Fatal("valid binding rejected")
	}
	for _, mutate := range []func(*workspaceevents.SettleWorkspacePreparationBlockedCommand){
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) { c.Original.RequestID = "other" },
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.SkillRegistryFacts.WorkspaceID = common.WorkspaceID("other")
		},
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.SkillRegistryFacts.WorkspaceRevision++
		},
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.SkillRegistryFacts.SnapshotHash = "other"
			c.SkillRegistryFacts.ReobservedSnapshotHash = "other"
		},
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.SkillRegistryFacts.StartedAt = c.SkillRegistryFacts.StartedAt.Add(-time.Hour)
		},
	} {
		bad := c
		f := facts.Clone()
		bad.SkillRegistryFacts = &f
		mutate(&bad)
		if e, reason := owners.workspace.bindOriginalPreparationSkillRegistry(ctx, bad); e != nil || reason != "skill_registry_observation_unavailable_not_saved" {
			t.Fatal("caller bypassed original identity")
		}
	}
}
