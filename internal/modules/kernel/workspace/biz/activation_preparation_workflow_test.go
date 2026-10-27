package workspace

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationWorkflowTypedApplicationBlockedReadbackAndReopen(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	w := seedPreparationEligible(t, owners, "ws-preparation-workflow")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
	request := workorchmanagementcontract.PreparationCheckRequest{RequestID: "check-original", ExpectedWorkspaceRevision: w.Revision}
	operation, err := b.CheckStartupPreparation(ctx, w.ID, request, false)
	if err != nil || operation.ValidateFor(string(w.ID), request) != nil {
		t.Fatalf("typed operation: %v %+v", err, operation)
	}
	view := operation.Preparation
	if view.OperationState != "blocked" || view.CurrentValidity != "not_evaluated" || view.WorkspaceRevision != w.Revision+2 || view.Record.Version != workspace.WorkspacePreparationWorkflowVersion || len(view.Record.Attempts) != 1 {
		t.Fatalf("incorrect operation/state axes: %+v", view)
	}
	a := view.Record.Attempts[0]
	if a.CreationEpoch != w.PreparationFence.CreationEpoch || a.PreparationEpoch != w.PreparationFence.PreparationEpoch || len(a.Checks) < 10 {
		t.Fatal("missing creation/epoch/required inventory")
	}
	for _, check := range a.Checks {
		wantReason := "check_pipeline_incomplete"
		wantState := "blocked"
		if check.ID == "exact_seal" {
			wantState, wantReason = "passed", "seal_observed_remaining_checks"
		}
		if check.ID == "persistence_integrity" {
			wantState, wantReason = "passed", "persistence_binding_observed_remaining_checks"
		}
		if check.ID == "mcp_selection" {
			wantReason = "mcp_registry_observation_unavailable_not_saved"
		}
		if check.ID == "memory_selection" {
			wantReason = "memory_inventory_unavailable_not_saved"
		}
		if check.ID == "provider_cache" {
			wantReason = "provider_cache_unavailable"
		}
		if check.ID == "first_activation_no_run" {
			wantReason = "run_history_unavailable"
		}
		if check.ID == "directory_boundary" {
			wantReason = "directory_observation_unavailable"
		}
		if check.ID == "managed_configuration" {
			wantReason = "managed_config_observation_unavailable"
		}
		if check.ID == "soul_selection" {
			wantReason = "soul_observation_unavailable_not_saved"
		}
		if check.ID == "skill_selection" {
			wantReason = "skill_registry_observation_unavailable_not_saved"
		}
		if check.State != wantState || check.Reason != wantReason {
			t.Fatal("unconnected fact promoted to success")
		}
	}
	for _, recoverOriginal := range []bool{false, true, false} {
		again, err := b.CheckStartupPreparation(ctx, w.ID, request, recoverOriginal)
		if err != nil || !reflect.DeepEqual(operation, again) {
			t.Fatalf("duplicate changed original: %v", err)
		}
	}
	stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	if stored.ActiveRunID != "" || !reflect.DeepEqual(stored.PreparationFence, w.PreparationFence) || !reflect.DeepEqual(stored.StartupInput, w.StartupInput) || !reflect.DeepEqual(stored.ActivationAdmission, w.ActivationAdmission) || !reflect.DeepEqual(stored.DefinitionAuthoring, w.DefinitionAuthoring) {
		t.Fatal("preparation mutated execution/sealed authoring authority")
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	b = &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
	reopened, err := b.ReadStartupPreparationView(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(view, reopened) {
		t.Fatalf("file owner reopen lost original operation: %v", err)
	}
	// A new explicit check has a new identity/revision; it preserves history.
	request.RequestID, request.ExpectedWorkspaceRevision = "check-next", reopened.WorkspaceRevision
	next, err := b.CheckStartupPreparation(ctx, w.ID, request, false)
	if err != nil || len(next.Preparation.Record.Attempts) != 2 || !reflect.DeepEqual(next.Preparation.Record.Attempts[0], a) {
		t.Fatalf("new check rewrote old attempt: %v", err)
	}
}

type preparationWriteCutRepository struct {
	repository.WorkspaceRepository
	writes       int
	failAt       int
	persistFirst bool
}

func (r *preparationWriteCutRepository) CompareAndSwap(ctx context.Context, w *workspace.Workspace, revision uint64) error {
	r.writes++
	if r.writes == r.failAt {
		if r.persistFirst {
			if err := r.WorkspaceRepository.CompareAndSwap(ctx, w, revision); err != nil {
				return err
			}
		}
		return errors.New("simulated response loss")
	}
	return r.WorkspaceRepository.CompareAndSwap(ctx, w, revision)
}

func TestPreparationWorkflowDurableCutsRecoverOriginalWithoutReplay(t *testing.T) {
	for _, scenario := range []string{"accepted_reply_lost", "completion_write_failed", "completed_reply_lost"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer func() { owners.close() }()
			w := seedPreparationEligible(t, owners, "ws-preparation-cut")
			cut := &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 1, persistFirst: true}
			if scenario != "accepted_reply_lost" {
				cut.failAt = 2
				cut.persistFirst = scenario == "completed_reply_lost"
			}
			owners.workspace.workspaces = cut
			command := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original-cut", ExpectedWorkspaceRevision: w.Revision}
			if _, err := owners.workspace.checkWorkspacePreparation(ctx, command, false); err == nil {
				t.Fatal("lost response reported success")
			}
			before, err := owners.workspace.queryWorkspacePreparationState(ctx, workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: w.ID})
			if err != nil || before.Record == nil || len(before.Record.Attempts) != 1 {
				t.Fatalf("GET lost accepted original: %v", err)
			}
			wantState := "accepted"
			if scenario == "completed_reply_lost" {
				wantState = "blocked"
			}
			if before.State != wantState || cut.writes != cut.failAt {
				t.Fatal("GET retried or settled operation")
			}
			duplicate, err := owners.workspace.checkWorkspacePreparation(ctx, command, false)
			if err != nil || duplicate.State.State != wantState || cut.writes != cut.failAt {
				t.Fatal("ordinary duplicate resumed accepted work")
			}
			other := command
			other.RequestID, other.ExpectedWorkspaceRevision = "new-key", before.WorkspaceRevision
			if scenario != "completed_reply_lost" {
				if _, err := owners.workspace.checkWorkspacePreparation(ctx, other, false); !errors.Is(err, common.ErrConflict) {
					t.Fatal("new identity bypassed unresolved attempt")
				}
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			result, err := owners.workspace.checkWorkspacePreparation(ctx, command, true)
			if err != nil || result.State.State != "blocked" || result.RequestID != command.RequestID || len(result.State.Record.Attempts) != 1 {
				t.Fatalf("explicit original recovery failed: %v", err)
			}
			a := result.State.Record.Attempts[0]
			if a.PayloadHash != command.PayloadHash() || a.ExpectedWorkspaceRevision != command.ExpectedWorkspaceRevision {
				t.Fatal("recovery replaced original input")
			}
			if scenario != "completed_reply_lost" && a.Checks[0].Reason != "original_check_interrupted" {
				t.Fatal("interrupted checks pretended to have been observed")
			}
			revision := result.State.WorkspaceRevision
			result, err = owners.workspace.checkWorkspacePreparation(ctx, command, true)
			if err != nil || result.State.WorkspaceRevision != revision {
				t.Fatal("repeated recovery wrote again")
			}
		})
	}
}

func TestPreparationWorkflowRejectsIdentityDriftAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-identity")
	command := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original", ExpectedWorkspaceRevision: w.Revision}
	stale := command
	stale.ExpectedWorkspaceRevision--
	if _, err := owners.workspace.checkWorkspacePreparation(ctx, stale, false); !errors.Is(err, common.ErrConflict) {
		t.Fatal("stale first attempt accepted")
	}
	if _, err := owners.workspace.checkWorkspacePreparation(ctx, command, true); !errors.Is(err, common.ErrNotFound) {
		t.Fatal("recovery created a missing original")
	}
	result, err := owners.workspace.checkWorkspacePreparation(ctx, command, false)
	if err != nil {
		t.Fatal(err)
	}
	command.ExpectedWorkspaceRevision = result.State.WorkspaceRevision
	if _, err := owners.workspace.checkWorkspacePreparation(ctx, command, false); !errors.Is(err, common.ErrConflict) {
		t.Fatal("same key different payload accepted")
	}
	stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	for _, scenario := range []string{"erase", "rewrite_binding", "rewrite_required", "fake_ready", "metadata_update"} {
		t.Run(scenario, func(t *testing.T) {
			next := *stored
			record := stored.StartupPreparation.Clone()
			next.StartupPreparation = &record
			switch scenario {
			case "erase":
				next.StartupPreparation = nil
			case "rewrite_binding":
				record.Attempts[0].Binding.ManifestHash = "forged"
			case "rewrite_required":
				record.Attempts[0].Checks[0].Required = false
			case "fake_ready":
				record.Attempts[0].State = "ready"
			case "metadata_update":
				record.Attempts[0].CompletedAt = record.Attempts[0].CompletedAt.Add(time.Second)
				if err := owners.repositories.Workspaces.Update(ctx, &next); !errors.Is(err, common.ErrConflict) {
					t.Fatal("non-CAS preparation write accepted")
				}
				return
			}
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &next, stored.Revision); !errors.Is(err, common.ErrConflict) {
				t.Fatal("forged/erased completed history accepted")
			}
		})
	}
	clone := stored.StartupPreparation.Clone()
	clone.Attempts[0].Checks[0].Owner = "caller"
	again, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	if !reflect.DeepEqual(stored, again) {
		t.Fatal("clone or rejected transition mutated persistence")
	}
}

func TestPreparationWorkflowConcurrentOriginalAndDriftedRecovery(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-race")
	command := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "race-original", ExpectedWorkspaceRevision: w.Revision}
	var wg sync.WaitGroup
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := owners.workspace.checkWorkspacePreparation(ctx, command, false)
			if err != nil && !errors.Is(err, common.ErrConflict) {
				t.Errorf("concurrent original: %v", err)
			}
		}()
	}
	wg.Wait()
	stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	if stored.StartupPreparation == nil || len(stored.StartupPreparation.Attempts) != 1 || stored.Revision != w.Revision+2 {
		t.Fatal("concurrent original duplicated CAS/attempt")
	}
	// Exercise accepted cut plus metadata drift using the real private reducer.
	read, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: stored.Revision})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := startuppreparation.CompilePreparationManifest(read.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	command.RequestID, command.ExpectedWorkspaceRevision = "drift-original", stored.Revision
	_, accepted, err := owners.workspace.acceptPreparation(ctx, workspacePreparationAccepted{Command: command, Snapshot: read.Snapshot, Manifest: manifest, At: time.Now().UTC()})
	if err != nil || !accepted {
		t.Fatalf("accepted cut: %v", err)
	}
	current, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	current.Title = "metadata changed"
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
		t.Fatal(err)
	}
	result, err := owners.workspace.checkWorkspacePreparation(ctx, command, true)
	if err != nil || result.State.CurrentValidity != "stale" || result.State.Record.Attempts[1].Checks[0].Reason != "binding_changed" {
		t.Fatalf("drift did not remain stale/blocked: %v %+v", err, result)
	}
}
