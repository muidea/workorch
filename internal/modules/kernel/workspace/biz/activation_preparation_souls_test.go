package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	soulowner "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type preparationSoulRepository struct {
	repository.SoulRepository
	reads map[common.SoulID]int
	drift bool
}

func TestPreparationSoulsRejectsTypedReplyAndCompleteRecordDrift(t *testing.T) {
	for _, scenario := range []string{"wrong_type", "nil_record", "wrong_id", "bad_hash", "zero_revision", "unknown_provenance", "owner_error", "same_hash_record_drift"} {
		t.Run(scenario, func(t *testing.T) {
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-soul-reply")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			observer := magicevent.NewSimpleObserver(soulevents.CommandDestination, owners.hub)
			calls := 0
			if err := observer.Subscribe(soulevents.TopicSoulCommandGet, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				cmd, ok := ev.Data().(soulevents.GetSoulCommand)
				deadline, bounded := ev.Context().Deadline()
				if !ok || cmd.SoulID != "system-soul" || ev.Source() != "application/workorchd/appservice" || !bounded || time.Until(deadline) > 3*time.Second {
					t.Error("wrong source, identity or budget")
				}
				if scenario == "wrong_type" {
					result.Set("wrong reply", nil)
					return
				}
				if scenario == "nil_record" {
					result.Set(soulevents.GetSoulResult{}, nil)
					return
				}
				if scenario == "owner_error" {
					result.Set(soulevents.GetSoulResult{}, cd.NewError(cd.Unexpected, "original owner error"))
					return
				}
				item := &soulmodel.Soul{ID: "system-soul", Revision: 1, Name: "name", SoulMD: "Original instruction\n", SourceProvenance: soulmodel.SoulSourceUserManaged, AllowedSkillRefs: []string{"same"}}
				switch scenario {
				case "wrong_id":
					item.ID = "other"
				case "zero_revision":
					item.Revision = 0
				case "unknown_provenance":
					item.SourceProvenance = "unknown"
				case "same_hash_record_drift":
					if calls == 2 {
						item.AllowedSkillRefs = nil
						item.AllowedMCPServerRefs = []string{"same"}
					}
				}
				item.ContentHash = soulmodel.StableSoulContentHash(item)
				if scenario == "bad_hash" {
					item.ContentHash = "wrong"
				}
				result.Set(soulevents.GetSoulResult{Soul: item}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer observer.Unsubscribe(soulevents.TopicSoulCommandGet)
			cfg := workorchconfig.Config{Runtime: workorchconfig.RuntimeConfig{DefaultSoulID: "system-soul"}}
			ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) { return cfg, nil })
			application := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			value, err := application.ObservePreparationSouls(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
			if err == nil || value.Reobserved || value.SealBindingCurrent {
				t.Fatal("bad owner reply accepted")
			}
			wantCalls := 1
			if scenario == "same_hash_record_drift" {
				wantCalls = 2
				if value.FailureCode != "soul_drift" || len(value.Souls) != 1 || len(value.Souls[0].AllowedSkillRefs) != 1 {
					t.Fatal("complete-record drift lost original observation")
				}
			}
			if calls != wantCalls {
				t.Fatal("unexpected retry or extra lookup")
			}
		})
	}
}

func (r *preparationSoulRepository) Get(ctx context.Context, id common.SoulID) (*soulmodel.Soul, error) {
	item, err := r.SoulRepository.Get(ctx, id)
	r.reads[id]++
	if err == nil && r.drift && r.reads[id] == 2 {
		item.SoulMD = "new original instruction\n"
		item.Revision++
		item.ContentHash = soulmodel.StableSoulContentHash(item)
	}
	return item, err
}

func TestPreparationSoulsExactOwnerObservationAndReopen(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligibleWithConfiguration(t, owners, "ws-soul-observation", t.TempDir(), func(w *workspace.Workspace) {
		w.DefaultSoulID = "workspace-soul"
		w.SoulBindings = map[string]common.SoulID{"execution": "execution-soul"}
	})
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "souls.json")
	repo := filerepository.NewSoulRepository(path)
	for _, id := range []common.SoulID{"workspace-soul", "execution-soul"} {
		item := &soulmodel.Soul{ID: id, Revision: 1, Name: string(id), SoulMD: "Original 原文\n", SourceProvenance: soulmodel.SoulSourceUserManaged, AllowedSkillRefs: []string{"*"}}
		item.ContentHash = soulmodel.StableSoulContentHash(item)
		if err := repo.Create(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	beforeBytes := preparationSoulTree(t, path)
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}
	application := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	for round := 0; round < 2; round++ {
		observedRepo := &preparationSoulRepository{SoulRepository: filerepository.NewSoulRepository(path), reads: map[common.SoulID]int{}}
		owner, err := soulowner.New(owners.hub, nil, soulowner.Dependencies{Souls: observedRepo})
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		cfg := workorchconfig.Config{Runtime: workorchconfig.RuntimeConfig{DefaultSoulID: "unused-system-soul"}}
		ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) { return cfg, nil })
		value, err := application.ObservePreparationSouls(ctx, command)
		if err != nil || !value.Reobserved || !value.SealBindingCurrent || len(value.Selections) != 6 || len(value.Souls) != 2 || value.DefaultLookupSoulID != "workspace-soul" {
			t.Fatalf("exact soul observation failed: %s %v", value.FailureCode, err)
		}
		for _, s := range value.Selections {
			if s.Stage == "execution" {
				if s.SoulID != "execution-soul" || s.Source != "workspace_stage_binding" {
					t.Fatal("stage priority lost")
				}
			} else if s.SoulID != "workspace-soul" || s.Source != "workspace_default" {
				t.Fatal("workspace default priority lost")
			}
		}
		if observedRepo.reads["workspace-soul"] != 2 || observedRepo.reads["execution-soul"] != 2 || observedRepo.reads["unused-system-soul"] != 0 {
			t.Fatal("wrong owner selection or retry")
		}
		value.Souls[0].AllowedSkillRefs[0] = "caller mutation"
		observedRepo.reads = map[common.SoulID]int{}
		observedRepo.drift = true
		value, err = application.ObservePreparationSouls(ctx, command)
		if err == nil || value.FailureCode != "soul_drift" || value.Reobserved || value.SealBindingCurrent || len(value.Souls) != 2 {
			t.Fatal("soul drift accepted or first observations discarded")
		}
		if err := owner.Teardown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	afterBytes := preparationSoulTree(t, path)
	if !reflect.DeepEqual(beforeBytes, afterBytes) {
		t.Fatal("read-only observation changed Soul persistence")
	}
	stored, err := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if err != nil || !reflect.DeepEqual(stored, w) {
		t.Fatal("observation changed Workspace")
	}
}

func preparationSoulTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	ret := map[string][]byte{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			ret[rel+"/"] = nil
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ret[rel] = data
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return ret
}

func TestPreparationSoulsUnboundAndRequiredReadFailures(t *testing.T) {
	for _, scenario := range []string{"unbound", "no_loader", "config_error", "system_default", "missing_soul", "system_drift", "seal_drift", "cancelled", "parent_scope"} {
		t.Run(scenario, func(t *testing.T) {
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-soul-failure")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			cfg := workorchconfig.Config{}
			if scenario == "missing_soul" || scenario == "system_default" {
				cfg.Runtime.DefaultSoulID = "system-soul"
			}
			path := filepath.Join(t.TempDir(), "souls.json")
			repo := filerepository.NewSoulRepository(path)
			if scenario == "system_default" {
				item := &soulmodel.Soul{ID: "system-soul", Revision: 3, Name: "system soul", SoulMD: "System instruction\n", SourceProvenance: soulmodel.SoulSourceBuiltinSeed}
				item.ContentHash = soulmodel.StableSoulContentHash(item)
				if err := repo.Create(context.Background(), item); err != nil {
					t.Fatal(err)
				}
			}
			owner, err := soulowner.New(owners.hub, nil, soulowner.Dependencies{Souls: repo})
			if err != nil {
				t.Fatal(err)
			}
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			defer owner.Teardown(context.Background())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			loads := 0
			if scenario != "no_loader" {
				ctx = configcore.ContextWithLoader(ctx, func() (workorchconfig.Config, error) {
					loads++
					if scenario == "config_error" {
						return cfg, errors.New("original config failure")
					}
					if scenario == "system_drift" && loads == 2 {
						cfg.Runtime.DefaultSoulID = "new-default"
					}
					return cfg, nil
				})
			}
			if scenario == "seal_drift" {
				owners.workspace.workspaces = &preparationRevisionReadRepository{WorkspaceRepository: owners.workspace.workspaces, driftAt: 5}
			}
			if scenario == "cancelled" {
				cancel()
			}
			if scenario == "parent_scope" {
				ctx, _ = common.BindRuntimeWorkspaceContext(ctx, "other")
			}
			application := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			value, err := application.ObservePreparationSouls(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
			valid := scenario == "unbound" || scenario == "system_default"
			if (err == nil) != valid || value.SealBindingCurrent != valid || value.Reobserved != valid {
				t.Fatalf("wrong result: %s %v", value.FailureCode, err)
			}
			if scenario == "unbound" {
				if len(value.Souls) != 0 || len(value.Selections) != 6 {
					t.Fatal("unbound observation invented resources")
				}
				for _, s := range value.Selections {
					if s.Source != "unbound" || s.SoulID != "" {
						t.Fatal("unbound stage hidden")
					}
				}
			}
			if scenario == "system_default" {
				if len(value.Souls) != 1 || value.Souls[0].Revision != 3 || value.Souls[0].SourceProvenance != soulmodel.SoulSourceBuiltinSeed {
					t.Fatal("system default identity lost")
				}
				for _, s := range value.Selections {
					if s.Source != "system_default" || s.SoulID != "system-soul" {
						t.Fatal("system default not observed")
					}
				}
			}
			if !valid && value.FailureCode == "" {
				t.Fatal("failure has no reason")
			}
		})
	}
}
