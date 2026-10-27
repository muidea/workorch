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
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationDirectoryProductionMaterialOriginalReopenAndNoReplay(t *testing.T) {
	for _, scenario := range []string{"existing", "missing", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, filepath.Join(root, "owner-data"))
			defer owners.close()
			delivery := t.TempDir()
			internal := t.TempDir()
			if scenario == "missing" {
				internal = filepath.Join(internal, "not-created")
			}
			if scenario == "symlink" {
				link := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(internal, link); err != nil {
					t.Fatal(err)
				}
				internal = link
			}
			w := seedPreparationEligibleWithConfiguration(t, owners, "ws-directory-material", delivery, func(w *workspace.Workspace) { w.InternalDirectory = internal })
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			fileOwner, err := filebiz.New(owners.hub, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			defer fileOwner.Teardown(ctx)
			var queries atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, "miss", &queries, func() {
				accepted, err := owners.repositories.Workspaces.Get(ctx, w.ID)
				if err != nil {
					t.Error(err)
					return
				}
				state, err := preparationStateFromPersistence(accepted, time.Now().UTC())
				if err != nil || state.CurrentValidity == "stale" {
					t.Errorf("accepted binding already stale: %s %v", state.CurrentValidity, err)
				}
				_, err = owners.workspace.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: accepted.Revision})
				if err != nil {
					t.Errorf("accepted seal not queryable: %v", err)
				}
			})
			b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			operation, err := b.CheckStartupPreparation(ctx, w.ID, request, false)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || len(stored.PreparationDirectoryEvidence) != 1 {
				t.Fatal("owner material missing", err)
			}
			e := stored.PreparationDirectoryEvidence[0]
			a := operation.Preparation.Record.Attempts[0]
			if a.State != "blocked" || a.DirectoryEvidenceHash != e.ContentHash || workspace.ValidatePreparationEvidencePersistence(stored) != nil || e.ObservedWorkspaceRevision != w.Revision+1 || e.Internal.Path != internal || e.Deliverable.Path != delivery || !e.Deliverable.Complete {
				t.Fatal("material lost original exact scope")
			}
			want := "directory_observed_remaining_checks"
			if scenario != "existing" {
				want = "directory_observation_incomplete"
				if e.Internal.Complete || e.Internal.DiagnosticBytes == 0 {
					t.Fatal("filesystem failure not preserved")
				}
			} else if !e.Internal.Complete {
				t.Fatal("real directory not observed")
			}
			for _, check := range a.Checks {
				if check.State != expectedPreparationCheckState(check.ID, check.Reason) {
					t.Fatal("incorrect per-check judgment", check)
				}
				if check.ID == "directory_boundary" && check.Reason != want {
					t.Fatalf("directory reason = %s, want %s", check.Reason, want)
				}
			}
			public, _ := json.Marshal(operation)
			var publicObject map[string]json.RawMessage
			if json.Unmarshal(public, &publicObject) != nil {
				t.Fatal("invalid public DTO")
			}
			// Ordinary DTO only projects the hash, never material or raw paths.
			for _, secret := range []string{internal, delivery, "DiagnosticSHA256", "PreparationDirectoryEvidence"} {
				if containsJSONText(public, secret) {
					t.Fatal("private File material leaked to public DTO")
				}
			}
			// Change external filesystem after the observation. Reads/duplicates
			// must return the immutable history, not silently reread or repair it.
			if err := os.WriteFile(filepath.Join(delivery, "later"), []byte("later"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, recoverOriginal := range []bool{false, true} {
				again, err := b.CheckStartupPreparation(ctx, w.ID, request, recoverOriginal)
				if err != nil || !reflect.DeepEqual(again, operation) {
					t.Fatal("original material replaced on replay", err)
				}
			}
			if _, err := b.ReadStartupPreparationView(ctx, w.ID); err != nil {
				t.Fatal(err)
			}
			if queries.Load() != 2 {
				t.Fatal("GET/replay/recover repeated cache/history")
			}
			// Independent repository reopen: exact original diagnostic bytes/hash.
			reopened := openFileDefinitionAdmissionOwners(t, filepath.Join(root, "owner-data"))
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(read.PreparationDirectoryEvidence, stored.PreparationDirectoryEvidence) {
				t.Fatal("reopen lost material", err)
			}
			inventory, _ := owners.repositories.Runs.List(ctx)
			if len(inventory) != 0 {
				t.Fatal("preparation created a Run")
			}
		})
	}
}

// Independent expected outcome table: a point-in-time file check is separate
// from the operation (which must still be blocked).
func expectedPreparationCheckState(id, reason string) string {
	if id == "first_activation_no_run" && reason == "run_history_empty_final_fence_observed" {
		return "passed"
	}
	if id == "exact_seal" && reason == "seal_observed_remaining_checks" || id == "persistence_integrity" && reason == "persistence_binding_observed_remaining_checks" {
		return "passed"
	}
	if id == "soul_selection" && reason == "souls_observed_remaining_checks" {
		return "passed"
	}
	if id == "directory_boundary" && reason == "directory_observed_remaining_checks" || id == "managed_configuration" && reason == "managed_configs_observed_remaining_checks" || len(id) > len("resource/") && id[:len("resource/")] == "resource/" && reason == "resource_observed_remaining_checks" {
		return "passed"
	}
	if id == "directory_boundary" && reason == "directory_permissions_denied" || id == "managed_configuration" && (reason == "managed_config_content_invalid" || reason == "managed_config_content_conflict") || len(id) > len("resource/") && id[:len("resource/")] == "resource/" && reason == "resource_content_changed" {
		return "failed"
	}
	return "blocked"
}

func containsJSONText(data []byte, value string) bool {
	encoded, _ := json.Marshal(value)
	return len(encoded) >= 2 && bytes.Contains(data, encoded[1:len(encoded)-1])
}

func TestPreparationDirectoryRecoveryDoesNotObserveInterruptedOriginal(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligibleInDirectory(t, owners, "ws-directory-interrupted", t.TempDir())
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original", ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(context.Background(), c, false, true); err != nil {
		t.Fatal(err)
	}
	result, err := owners.workspace.beginWorkspacePreparation(context.Background(), c, true, true)
	if err != nil || result.State.Record.Attempts[0].DirectoryEvidenceHash != "" || result.State.Record.Attempts[0].ResourceEvidenceHash != "" || result.State.Record.Attempts[0].ManagedConfigEvidenceRef != "" {
		t.Fatal("interrupted original invented material", err)
	}
	stored, _ := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if len(stored.PreparationDirectoryEvidence) != 0 || len(stored.PreparationResourceEvidence) != 0 || len(stored.PreparationManagedConfigEvidence) != 0 {
		t.Fatal("recovery observed or archived new File facts")
	}
}

type preparationDirectoryDriftRepository struct {
	repository.WorkspaceRepository
	writes int
}

func (r *preparationDirectoryDriftRepository) CompareAndSwap(ctx context.Context, w *workspace.Workspace, revision uint64) error {
	r.writes++
	if r.writes == 2 {
		current, err := r.WorkspaceRepository.Get(ctx, w.ID)
		if err != nil {
			return err
		}
		current.Title = "independent metadata change"
		if err := r.WorkspaceRepository.CompareAndSwap(ctx, current, current.Revision); err != nil {
			return err
		}
	}
	return r.WorkspaceRepository.CompareAndSwap(ctx, w, revision)
}

func TestPreparationDirectoryMaterialAtomicCutsAndDrift(t *testing.T) {
	for _, scenario := range []string{"write_failed", "reply_lost", "revision_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			w := seedPreparationEligibleWithConfiguration(t, owners, "ws-directory-cut", t.TempDir(), func(w *workspace.Workspace) { w.InternalDirectory = t.TempDir() })
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			fileOwner, err := filebiz.New(owners.hub, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			defer fileOwner.Teardown(ctx)
			var calls atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, "miss", &calls, nil)
			if scenario == "revision_drift" {
				owners.workspace.workspaces = &preparationDirectoryDriftRepository{WorkspaceRepository: owners.workspace.workspaces}
			} else {
				owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 2, persistFirst: scenario == "reply_lost"}
			}
			b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
			r := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			_, err = b.CheckStartupPreparation(ctx, w.ID, r, false)
			if scenario != "revision_drift" && err == nil {
				t.Fatal("ambiguous/failed CAS reported success")
			}
			if scenario == "revision_drift" && err != nil {
				t.Fatal(err)
			}
			stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			a := stored.StartupPreparation.Attempts[0]
			if scenario == "write_failed" {
				if len(stored.PreparationDirectoryEvidence) != 0 || a.DirectoryEvidenceHash != "" || a.State != workspace.WorkspacePreparationAccepted {
					t.Fatal("material/reference partially committed")
				}
			} else if len(stored.PreparationDirectoryEvidence) != 1 || a.DirectoryEvidenceHash == "" || a.State != workspace.WorkspacePreparationBlocked {
				t.Fatal("completed CAS lost material/reference")
			}
			if scenario == "revision_drift" {
				state, err := b.ReadStartupPreparationView(ctx, w.ID)
				if err != nil || state.CurrentValidity != "stale" {
					t.Fatal("drifted evidence treated as current", err)
				}
				for _, check := range a.Checks {
					if check.Reason != "binding_changed" {
						t.Fatal("late observed credit survived revision drift")
					}
				}
			}
			if _, err := b.CheckStartupPreparation(ctx, w.ID, r, false); err != nil {
				t.Fatal(err)
			}
			if _, err := b.CheckStartupPreparation(ctx, w.ID, r, true); err != nil {
				t.Fatal(err)
			}
			again, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			if !reflect.DeepEqual(stored.PreparationDirectoryEvidence, again.PreparationDirectoryEvidence) || calls.Load() != 2 {
				t.Fatal("recovery repeated observation or replaced raw material")
			}
			if scenario == "write_failed" && again.StartupPreparation.Attempts[0].Checks[0].Reason != "original_check_interrupted" {
				t.Fatal("uncommitted observation invented on recovery")
			}
			// Missing creation fence cannot be repaired by this material path.
			bad := *again
			bad.PreparationFence = nil
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &bad, again.Revision); err != common.ErrConflict {
				t.Fatal("preparation discarded creation fence", err)
			}
		})
	}
}
