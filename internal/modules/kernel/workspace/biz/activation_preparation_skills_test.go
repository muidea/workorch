package workspace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	skillowner "github.com/muidea/workorch/internal/modules/blocks/skill/biz"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func preparationSkillFixture() *skillmodel.Skill {
	item := &skillmodel.Skill{ID: "registry-skill", Name: "name", Version: "v1", Instruction: "原始指令\n第二行", TriggerRules: []string{"go"}}
	item.Provenance = workorchskillcontract.Provenance{Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal, ContentHash: workorchskillcontract.StableContentHash(item.Content())}
	return item
}

type preparationSkillListRepository struct {
	repository.SkillRepository
	reads int
}

func (r *preparationSkillListRepository) List(ctx context.Context) ([]*skillmodel.Skill, error) {
	r.reads++
	return r.SkillRepository.List(ctx)
}

func TestPreparationSkillRegistryRealOwnersFileReopenAndNoWrites(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-registry-observation")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "skills.json")
	repo := filerepository.NewSkillRepository(path)
	if err := repo.Create(context.Background(), preparationSkillFixture()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}
	var epoch string
	for range 2 {
		observed := &preparationSkillListRepository{SkillRepository: filerepository.NewSkillRepository(path)}
		owner, err := skillowner.New(owners.hub, nil, skillowner.Dependencies{Skills: observed})
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		func() {
			defer owner.Teardown(context.Background())
			value, err := app.ObservePreparationSkillRegistry(context.Background(), command)
			if err != nil || !value.Reobserved || !value.SealBindingCurrent || value.FailureCode != "" || len(value.Catalog.Skills) != 1 || observed.reads != 2 {
				t.Fatalf("real observation: %+v %v", value, err)
			}
			if epoch == value.Catalog.OwnerEpoch {
				t.Fatal("reopened owner reused epoch")
			}
			epoch = value.Catalog.OwnerEpoch
			if !reflect.DeepEqual(value.Catalog.Skills[0], preparationSkillFixture()) {
				t.Fatal("original record changed")
			}
			value.Catalog.Skills[0].TriggerRules[0] = "caller"
			observed.reads = 0
			value, err = app.ObservePreparationSkillRegistry(context.Background(), command)
			if err != nil || value.Catalog.Skills[0].TriggerRules[0] != "go" || observed.reads != 2 {
				t.Fatal("caller mutation leaked or catalog cached")
			}
			original := owners.workspace.workspaces
			owners.workspace.workspaces = &preparationRevisionReadRepository{WorkspaceRepository: original, driftAt: 4}
			value, err = app.ObservePreparationSkillRegistry(context.Background(), command)
			owners.workspace.workspaces = original
			if err == nil || value.Reobserved || value.SealBindingCurrent || value.FailureCode != "seal_reobservation_failed" || len(value.Catalog.Skills) != 1 {
				t.Fatalf("seal drift accepted/lost facts: %+v %v", value, err)
			}
		}()
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("read/reopen changed persisted registry")
	}
	stored, err := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if err != nil || !reflect.DeepEqual(stored, w) {
		t.Fatal("observation wrote Workspace")
	}
}

func TestPreparationSkillRegistryRejectsBadRepliesAndExactDrift(t *testing.T) {
	for _, scenario := range []string{"wrong_type", "owner_error", "scope", "version", "hash", "nil_rows", "bad_content", "unsorted", "duplicate", "old_time", "future_time", "epoch_drift", "content_drift", "added_row", "second_error"} {
		t.Run(scenario, func(t *testing.T) {
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-registry-reply")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			observer := magicevent.NewSimpleObserver(skillevents.CommandDestination, owners.hub)
			calls := 0
			if err := observer.Subscribe(skillevents.TopicObserveRegistryCatalog, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				cmd, ok := ev.Data().(skillevents.ObserveRegistryCatalogCommand)
				deadline, bounded := ev.Context().Deadline()
				if !ok || cmd.WorkspaceID != w.ID || ev.Source() != applicationbiz.ModuleID || !bounded || time.Until(deadline) > 3*time.Second {
					t.Error("wrong source/scope/budget")
				}
				if scenario == "wrong_type" {
					result.Set("wrong reply", nil)
					return
				}
				if scenario == "owner_error" || scenario == "second_error" && calls == 2 {
					result.Set(nil, cd.NewError(cd.Unexpected, "original owner error"))
					return
				}
				value := skillevents.ObserveRegistryCatalogResult{Version: skillevents.RegistryCatalogVersion, WorkspaceID: w.ID, OwnerEpoch: "skill-preparation-epoch:" + strings.Repeat("A", 26), ObservedAt: time.Now().UTC(), Skills: []*skillmodel.Skill{preparationSkillFixture()}}
				switch scenario {
				case "scope":
					value.WorkspaceID = "other"
				case "version":
					value.Version = "other"
				case "nil_rows":
					value.Skills = nil
				case "bad_content":
					value.Skills[0].Instruction += " "
				case "duplicate":
					value.Skills = append(value.Skills, preparationSkillFixture())
				case "unsorted":
					item := preparationSkillFixture()
					item.ID = "a-skill"
					item.Provenance.ContentHash = workorchskillcontract.StableContentHash(item.Content())
					value.Skills = append(value.Skills, item)
				case "old_time":
					value.ObservedAt = value.ObservedAt.Add(-time.Hour)
				case "future_time":
					value.ObservedAt = value.ObservedAt.Add(time.Hour)
				case "epoch_drift":
					if calls == 2 {
						value.OwnerEpoch = "skill-preparation-epoch:" + strings.Repeat("B", 26)
					}
				case "content_drift":
					if calls == 2 {
						value.Skills[0].Instruction = "new instruction"
						value.Skills[0].Provenance.ContentHash = workorchskillcontract.StableContentHash(value.Skills[0].Content())
					}
				case "added_row":
					if calls == 2 {
						item := preparationSkillFixture()
						item.ID = "z-skill"
						item.Provenance.ContentHash = workorchskillcontract.StableContentHash(item.Content())
						value.Skills = append(value.Skills, item)
					}
				}
				value.CatalogHash, _ = skillevents.RegistryCatalogHash(value.Skills)
				if scenario == "hash" {
					value.CatalogHash = "wrong"
				}
				result.Set(value, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer observer.Unsubscribe(skillevents.TopicObserveRegistryCatalog)
			app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			value, err := app.ObservePreparationSkillRegistry(context.Background(), workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
			if err == nil || value.Reobserved || value.SealBindingCurrent {
				t.Fatal("bad catalog accepted")
			}
			want := 1
			if scenario == "epoch_drift" || scenario == "content_drift" || scenario == "added_row" || scenario == "second_error" {
				want = 2
				if len(value.Catalog.Skills) != 1 || value.Catalog.Skills[0].Instruction != preparationSkillFixture().Instruction {
					t.Fatal("original observation lost")
				}
			}
			if calls != want {
				t.Fatalf("calls %d want %d", calls, want)
			}
		})
	}
}

func TestPreparationSkillRegistryScopeAndCancellationBeforeIO(t *testing.T) {
	hub := magicevent.NewHub(4)
	defer hub.Terminate(context.Background())
	app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, hub, nil)}
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: common.WorkspaceID("ws"), ExpectedWorkspaceRevision: 1}
	if _, err := app.ObservePreparationSkillRegistry(nil, command); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if value, err := app.ObservePreparationSkillRegistry(ctx, command); !errors.Is(err, context.Canceled) || value.FailureCode != "cancelled" {
		t.Fatal("cancelled observation accepted")
	}
	bound, err := common.BindRuntimeWorkspaceContext(context.Background(), "other")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := app.ObservePreparationSkillRegistry(bound, command); err == nil || value.FailureCode != "scope_conflict" {
		t.Fatal("scope conflict reached owner I/O")
	}
}
