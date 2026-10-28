package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationInitializationProductionCreatesMissingPreservesExistingAndReopens(t *testing.T) {
	for _, scenario := range []string{"missing", "preserve", "conflict", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			storage := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, storage)
			internal := filepath.Join(t.TempDir(), "internal", "workspaces", "ws")
			if err := os.MkdirAll(internal, 0700); err != nil {
				t.Fatal(err)
			}
			w := seedManagedPreparation(t, owners, internal, "exact")
			startManagedPreparationOwners(t, owners)
			system := workorchmanagedconfig.SystemDirectory(internal)
			var preserved []byte
			if scenario == "preserve" {
				preserved = []byte("{\r\n\"custom\":\"user override\"}\n")
				writeManagedFixture(t, internal, preserved)
			}
			if scenario == "conflict" {
				preserved = []byte(`{"do-not-overwrite":true}`)
				writeManagedFixture(t, system, preserved)
			}
			if scenario == "symlink" {
				outside := filepath.Join(t.TempDir(), "canary")
				os.WriteFile(outside, []byte("original-canary"), 0600)
				os.MkdirAll(filepath.Join(internal, "mcp"), 0700)
				if err := os.Symlink(outside, filepath.Join(internal, "mcp", "config.json")); err != nil {
					t.Fatal(err)
				}
			}
			// Existing failed preparation history is immutable through initialization.
			original := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "old-blocked", ExpectedWorkspaceRevision: w.Revision}
			old, err := owners.workspace.checkWorkspacePreparation(ctx, original, false)
			if err != nil {
				t.Fatal(err)
			}
			w, _ = owners.repositories.Workspaces.Get(ctx, w.ID)
			b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "initialize-original", ExpectedWorkspaceRevision: w.Revision}
			operation, err := b.InitializeStartupPreparationResources(ctx, w.ID, request, false)
			if err != nil {
				x, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
				t.Fatalf("app error %v owner journal %+v", err, x.PreparationResourceInitializations)
			}
			want := "completed"
			if scenario == "conflict" || scenario == "symlink" {
				want = "blocked"
			}
			last := operation.Preparation.ResourceInitializations[0]
			if last.State != want || last.Attempts != 1 || operation.Preparation.WorkspaceRevision != w.Revision+2 || !reflect.DeepEqual(old.State.Record, operationToOwnerRecord(t, owners, w.ID)) {
				t.Fatalf("incorrect initialization/history: %+v", operation)
			}
			if _, err := os.Stat(filepath.Join(w.DeliverableDirectory, ".agents", "skills")); err != nil {
				t.Fatal(err)
			}
			if preserved != nil {
				root := internal
				if scenario == "conflict" {
					root = system
				}
				actual, _ := os.ReadFile(filepath.Join(root, "mcp", "config.json"))
				if !bytes.Equal(actual, preserved) {
					t.Fatal("overwrote existing config bytes")
				}
			}
			stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			if stored.ActiveRunID != "" || len(stored.PreparationResourceInitializations) != 1 || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
				t.Fatal("invalid owner journal")
			}
			raw, _ := json.Marshal(stored)
			if bytes.Contains(raw, []byte("PreparationResourceInitializations")) || bytes.Contains(raw, []byte("WriteDiagnostic")) {
				t.Fatal("raw archive leaked into Workspace JSON")
			}
			before, _ := os.Stat(filepath.Join(system, "mcp", "config.json"))
			replay, err := b.InitializeStartupPreparationResources(ctx, w.ID, request, false)
			if err != nil || !reflect.DeepEqual(operation, replay) {
				t.Fatal("ordinary duplicate changed original", err)
			}
			after, _ := os.Stat(filepath.Join(system, "mcp", "config.json"))
			if before != nil && after != nil && !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("duplicate wrote resources")
			}
			owners.close()
			reopened := openFileDefinitionAdmissionOwners(t, storage)
			defer reopened.close()
			read, err := reopened.workspace.queryWorkspacePreparationState(ctx, workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: w.ID})
			if err != nil || read.ResourceInitializations[0].State != want {
				t.Fatal("journal lost on reopen", err)
			}
			if scenario == "conflict" {
				if _, err := reopened.workspace.beginWorkspacePreparation(ctx, workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "cannot-bypass", ExpectedWorkspaceRevision: read.WorkspaceRevision}, false, true); err == nil {
					t.Fatal("pending initialization bypassed")
				}
			}
		})
	}
}
func operationToOwnerRecord(t *testing.T, o *fileDefinitionAdmissionOwners, id common.WorkspaceID) *workspace.WorkspacePreparationRecord {
	t.Helper()
	w, err := o.repositories.Workspaces.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return w.StartupPreparation
}

func TestPreparationInitializationCASLossRecoveryAndDrift(t *testing.T) {
	for _, scenario := range []string{"reserve_before", "reserve_after", "settle_before", "settle_after"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			storage := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, storage)
			internal := t.TempDir()
			w := seedManagedPreparation(t, owners, internal, "exact")
			startManagedPreparationOwners(t, owners)
			at := 1
			if scenario == "settle_before" || scenario == "settle_after" {
				at = 2
			}
			owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: at, persistFirst: scenario == "reserve_after" || scenario == "settle_after"}
			c := workspaceevents.InitializeWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original", ExpectedWorkspaceRevision: w.Revision}
			if _, err := owners.workspace.initializePreparationResources(ctx, c, false); err == nil {
				t.Fatal("lost CAS reported success")
			}
			stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			if scenario == "reserve_before" {
				if len(stored.PreparationResourceInitializations) != 0 {
					t.Fatal("unaccepted journal")
				}
				if _, err := os.Stat(filepath.Join(internal, "mcp", "config.json")); !os.IsNotExist(err) {
					t.Fatal("effect before reservation")
				}
				owners.close()
				return
			}
			before, _ := os.ReadFile(filepath.Join(internal, "mcp", "config.json"))
			// An ordinary replay only reconciles, it never repeats File work.
			replay, err := owners.workspace.initializePreparationResources(ctx, c, false)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "reserve_after" {
				if len(replay.State.ResourceInitializations) != 1 || replay.State.ResourceInitializations[0].State != "pending" {
					t.Fatal("lost reservation not pending")
				}
				if _, err := os.Stat(filepath.Join(internal, "mcp", "config.json")); !os.IsNotExist(err) {
					t.Fatal("replay performed File write")
				}
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, storage)
			defer owners.close()
			startManagedPreparationOwners(t, owners)
			recovered, err := owners.workspace.initializePreparationResources(ctx, c, true)
			if err != nil || recovered.State.ResourceInitializations[0].State != "completed" {
				x, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
				t.Fatalf("explicit recovery failed %v result %+v journal %+v", err, recovered, x.PreparationResourceInitializations)
			}
			actual, _ := os.ReadFile(filepath.Join(internal, "mcp", "config.json"))
			if len(before) > 0 && !bytes.Equal(before, actual) {
				t.Fatal("recovery overwrote original")
			}
			wrong := c
			wrong.ExpectedWorkspaceRevision++
			if _, err := owners.workspace.initializePreparationResources(ctx, wrong, true); err == nil {
				t.Fatal("accepted changed original revision")
			}
			state, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			state.PreparationResourceInitializations[0].Plan.InternalDirectory = t.TempDir()
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, state, state.Revision); err == nil {
				t.Fatal("allowed sealed plan replacement")
			}
		})
	}
}

type initializationCompletionFailureRepository struct {
	repository.WorkspaceRepository
	writes int
}

func (r *initializationCompletionFailureRepository) CompareAndSwap(ctx context.Context, w *workspace.Workspace, revision uint64) error {
	r.writes++
	if len(w.PreparationResourceInitializations) > 0 {
		a := w.PreparationResourceInitializations[len(w.PreparationResourceInitializations)-1].Attempts
		if len(a) > 0 && a[len(a)-1].Result != nil {
			return common.ErrConflict
		}
	}
	return r.WorkspaceRepository.CompareAndSwap(ctx, w, revision)
}
func TestPreparationInitializationLostCompletionsCannotResetBudgetOrBypassFence(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedManagedPreparation(t, owners, t.TempDir(), "exact")
	startManagedPreparationOwners(t, owners)
	failures := &initializationCompletionFailureRepository{WorkspaceRepository: owners.workspace.workspaces}
	owners.workspace.workspaces = failures
	c := workspaceevents.InitializeWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "bounded-original", ExpectedWorkspaceRevision: w.Revision}
	for i := 0; i < workspace.PreparationInitializationResultLimit; i++ {
		if _, err := owners.workspace.initializePreparationResources(context.Background(), c, i > 0); err == nil {
			t.Fatal("lost completion reported success")
		}
	}
	saved, _ := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if len(saved.PreparationResourceInitializations) != 1 || len(saved.PreparationResourceInitializations[0].Attempts) != 4 || saved.PreparationResourceInitializations[0].State() != "pending" {
		t.Fatal("lost calls did not consume persisted attempts")
	}
	writes := failures.writes
	if _, err := owners.workspace.initializePreparationResources(context.Background(), c, true); err == nil || failures.writes != writes {
		t.Fatal("exhausted recovery reset budget or wrote a new reservation")
	}
	different := c
	different.RequestID = "new-cannot-bypass"
	different.ExpectedWorkspaceRevision = saved.Revision
	if _, err := owners.workspace.initializePreparationResources(context.Background(), different, false); err == nil {
		t.Fatal("new identity bypassed original budget")
	}
	next := *saved
	next.PreparationFence = saved.PreparationFence.Clone()
	next.PreparationFence.PreparationEpoch++
	if workspace.ValidatePreparationRecordTransition(saved, &next, true) == nil {
		t.Fatal("fresh execution fence bypassed pending infrastructure transaction")
	}
	next = *saved
	next.DefaultSoulID = "different-sidecar"
	next.PreparationResourceInitializations = workspace.ClonePreparationInitializations(saved.PreparationResourceInitializations)
	next.PreparationResourceInitializations[0].CommittedWorkspaceRevision++
	if workspace.ValidatePreparationInitializationPreimage(&next, saved) == nil {
		t.Fatal("accepted a drifted complete Soul sidecar preimage")
	}
}

func TestPreparationInitializationRejectsSelfConsistentUndeclaredPlan(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedManagedPreparation(t, owners, t.TempDir(), "exact")
	startManagedPreparationOwners(t, owners)
	owners.workspace.workspaces = &initializationCompletionFailureRepository{WorkspaceRepository: owners.workspace.workspaces}
	c := workspaceevents.InitializeWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original-plan", ExpectedWorkspaceRevision: w.Revision}
	owners.workspace.initializePreparationResources(context.Background(), c, false)
	saved, _ := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	for _, change := range []func(*workspace.PreparationResourceInitialization){func(r *workspace.PreparationResourceInitialization) {
		r.Plan.Declarations[0].Path = "other/target.json"
	}, func(r *workspace.PreparationResourceInitialization) {
		r.Plan.Declarations[0].DefaultsJSON = []byte(`{"changed":true}`)
	}} {
		next := *saved
		next.PreparationResourceInitializations = workspace.ClonePreparationInitializations(saved.PreparationResourceInitializations)
		r := &next.PreparationResourceInitializations[0]
		change(r)
		r.Plan.BindingHash = r.BindingHash()
		if r.Validate(saved.ID) != nil {
			t.Fatal("fixture must have a self-consistent hash")
		}
		if workspace.ValidatePreparationInitializationPreimage(saved, &next) == nil {
			t.Fatal("hash authorized an undeclared target/body")
		}
	}
}
