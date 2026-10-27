package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	soulowner "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationSoulEvidenceOriginalSettlementRecoveryAndReopen(t *testing.T) {
	for _, scenario := range []string{"selected", "system_default", "unbound", "no_loader", "missing", "drift", "system_drift", "write_failed", "reply_lost", "revision_drift"} {
		t.Run(scenario, func(t *testing.T) {
			storage := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, storage)
			defer owners.close()
			w := seedPreparationEligibleWithConfiguration(t, owners, "ws-soul-archive", t.TempDir(), func(w *workspace.Workspace) {
				if scenario != "unbound" && scenario != "system_default" {
					w.DefaultSoulID = "selected"
				}
			})
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "souls.json")
			repo := filerepository.NewSoulRepository(path)
			original := &soulmodel.Soul{ID: "selected", Revision: 4, Name: "Original", SoulMD: "原始 Soul\n", SourceProvenance: soulmodel.SoulSourceUserManaged, AllowedSkillRefs: []string{"skill"}}
			original.ContentHash = soulmodel.StableSoulContentHash(original)
			if scenario != "missing" {
				if err := repo.Create(context.Background(), original); err != nil {
					t.Fatal(err)
				}
			}
			before := map[string][]byte{}
			if scenario != "missing" {
				before = preparationSoulTree(t, path)
			}
			observed := &preparationSoulRepository{SoulRepository: repo, reads: map[common.SoulID]int{}, drift: scenario == "drift"}
			owner, err := soulowner.New(owners.hub, nil, soulowner.Dependencies{Souls: observed})
			if err != nil {
				t.Fatal(err)
			}
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			defer owner.Teardown(context.Background())
			var calls atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, "miss", &calls, nil)
			ctx := context.Background()
			loads := 0
			if scenario != "no_loader" {
				ctx = configcore.ContextWithLoader(ctx, func() (workorchconfig.Config, error) {
					loads++
					cfg := workorchconfig.Config{}
					if scenario == "system_default" {
						cfg.Runtime.DefaultSoulID = "selected"
					}
					if scenario == "system_drift" && loads == 2 {
						cfg.Runtime.DefaultSoulID = "other"
					}
					return cfg, nil
				})
			}
			if scenario == "revision_drift" {
				owners.workspace.workspaces = &preparationDirectoryDriftRepository{WorkspaceRepository: owners.workspace.workspaces}
			}
			if scenario == "write_failed" || scenario == "reply_lost" {
				owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 2, persistFirst: scenario == "reply_lost"}
			}
			b := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			op, err := b.CheckStartupPreparation(ctx, w.ID, request, false)
			ambiguous := scenario == "write_failed" || scenario == "reply_lost"
			if (err != nil) != ambiguous {
				t.Fatal("unexpected settlement result", err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
				t.Fatal("invalid persisted Soul binding", err)
			}
			a := stored.StartupPreparation.Attempts[0]
			complete := scenario != "no_loader" && scenario != "missing" && scenario != "drift" && scenario != "system_drift" && scenario != "write_failed"
			if complete {
				if len(stored.PreparationSoulEvidence) != 1 || a.SoulEvidenceRef == "" {
					t.Fatal("missing complete original evidence")
				}
				e := stored.PreparationSoulEvidence[0]
				if e.Ref != a.SoulEvidenceRef || e.RequestID != request.RequestID || e.Facts.WorkspaceRevision != w.Revision+1 || e.Facts.ConfigurationHash != a.Binding.AuthoringConfigurationHash || len(e.Facts.Selections) != 6 {
					t.Fatal("original binding lost")
				}
				if scenario == "unbound" {
					if len(e.Facts.Souls) != 0 {
						t.Fatal("unbound invented Soul")
					}
				} else {
					if len(e.Facts.Souls) != 1 || !reflect.DeepEqual(e.Facts.Souls[0], original) || !reflect.DeepEqual(e.Facts.Souls, e.Facts.ReobservedSouls) {
						t.Fatal("complete records or nil/empty shape changed")
					}
				}
				public, _ := json.Marshal(op)
				ordinary, _ := json.Marshal(stored)
				for _, secret := range []string{"原始 Soul", e.ContentHash, `"ReobservedSouls":`, `"PreparationSoulEvidence":`} {
					if bytes.Contains(public, []byte(secret)) || bytes.Contains(ordinary, []byte(secret)) {
						t.Fatal("Soul private material leaked", secret)
					}
				}
			} else if len(stored.PreparationSoulEvidence) != 0 || a.SoulEvidenceRef != "" {
				t.Fatal("partial/failing observation archived as complete")
			}
			if scenario != "write_failed" {
				for _, check := range a.Checks {
					wantState := expectedPreparationCheckState(check.ID, check.Reason)
					if scenario == "unbound" && check.ID == "soul_selection" {
						wantState = "blocked"
					}
					if string(check.State) != wantState {
						t.Fatal("incorrect per-check judgment", check)
					}
					if check.ID == "soul_selection" {
						want := "soul_observation_unavailable_not_saved"
						if complete {
							want = "souls_observed_remaining_checks"
						}
						if scenario == "revision_drift" {
							want = "binding_changed"
						}
						if check.Reason != want {
							t.Fatal("wrong negative classification", check.Reason, want)
						}
					}
				}
			}
			reads, configLoads := observed.reads["selected"], loads
			for _, recoverOriginal := range []bool{false, true, false} {
				if _, err := b.CheckStartupPreparation(ctx, w.ID, request, recoverOriginal); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := b.ReadStartupPreparationView(ctx, w.ID); err != nil {
				t.Fatal(err)
			}
			reopened := openFileDefinitionAdmissionOwners(t, storage)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(stored.PreparationSoulEvidence, read.PreparationSoulEvidence) || observed.reads["selected"] != reads || loads != configLoads || calls.Load() != 2 {
				t.Fatal("recovery/reopen reobserved or replaced original evidence", err)
			}
			if scenario != "missing" && !reflect.DeepEqual(before, preparationSoulTree(t, path)) {
				t.Fatal("observation modified Soul owner")
			}
			if scenario == "selected" {
				// Completed history cannot be erased, rehashed or detached from its reference.
				for _, change := range []string{"erase", "ref", "rewrite"} {
					bad, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
					switch change {
					case "erase":
						bad.PreparationSoulEvidence = nil
					case "ref":
						bad.StartupPreparation.Attempts[0].SoulEvidenceRef = ""
					case "rewrite":
						e := &bad.PreparationSoulEvidence[0]
						e.Facts.Souls[0].SoulMD = "new"
						e.Facts.ReobservedSouls[0].SoulMD = "new"
						for _, s := range []*soulmodel.Soul{e.Facts.Souls[0], e.Facts.ReobservedSouls[0]} {
							s.ContentHash = soulmodel.StableSoulContentHash(s)
						}
						e.ContentHash = e.Hash()
					}
					if err := owners.repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
						t.Fatal("completed Soul history rewritten", change)
					}
				}
				if err := filepath.WalkDir(storage, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.Name() == "workspace.json" {
						info, err := d.Info()
						if err != nil {
							return err
						}
						if info.Mode().Perm() != 0600 {
							t.Error("private archive not 0600")
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			runs, _ := owners.repositories.Runs.List(ctx)
			if len(runs) != 0 || read.ActiveRunID != "" || !reflect.DeepEqual(read.PreparationFence, w.PreparationFence) {
				t.Fatal("Soul check changed execution authority")
			}
		})
	}
}

func TestPreparationSoulEvidenceRebindsOriginalOwnerInputs(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-soul-binding")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) { return workorchconfig.Config{}, nil })
	original := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original", ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, original, false, true); err != nil {
		t.Fatal(err)
	}
	b := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	observation, err := b.ObservePreparationSouls(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision + 1})
	if err != nil {
		t.Fatal(err)
	}
	c := workspaceevents.SettleWorkspacePreparationBlockedCommand{Original: original, ProviderCacheReason: "provider_cache_miss", RunHistoryReason: "run_history_empty_fence_not_final", SoulReason: "souls_observed_remaining_checks", SoulFacts: &observation.PreparationSoulFacts}
	if e, reason := owners.workspace.bindOriginalPreparationSouls(ctx, c); e == nil || reason != "souls_observed_remaining_checks" {
		t.Fatal("valid original facts not bound")
	}
	for _, mutate := range []func(*workspaceevents.SettleWorkspacePreparationBlockedCommand){
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) { c.Original.RequestID = "other" },
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.SoulFacts.ConfigurationHash = "different"
		},
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.SoulFacts.SnapshotHash = "different"
			c.SoulFacts.ReobservedSnapshotHash = "different"
		},
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) { c.SoulFacts.WorkspaceRevision++ },
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.SoulFacts.StartedAt = observation.StartedAt.Add(-time.Hour)
		},
	} {
		bad := c
		f := c.SoulFacts.Clone()
		bad.SoulFacts = &f
		mutate(&bad)
		if e, reason := owners.workspace.bindOriginalPreparationSouls(ctx, bad); e != nil || reason != "soul_observation_unavailable_not_saved" {
			t.Fatal("caller facts bypassed original owner binding")
		}
	}
	stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	if stored.Revision != w.Revision+1 || len(stored.PreparationSoulEvidence) != 0 {
		t.Fatal("binding query wrote archive")
	}
}
