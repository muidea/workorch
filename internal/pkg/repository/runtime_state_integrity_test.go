package repository_test

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Every physical repository and the production workspace-scoped path must use
// the same write gate. Wrong opaque identity is never published or retried as
// a successful revision; a later correct CAS still uses the original revision.
func TestRuntimeStateIntegrityPhysicalWriteBoundary(t *testing.T) {
	for name, factory := range map[string]func(string) repository.RunRepository{
		"memory": func(string) repository.RunRepository { return memory.NewRunRepository() },
		"file": func(root string) repository.RunRepository {
			return file.NewRunRepository(filepath.Join(root, "runs.json"))
		},
		"workspace": func(root string) repository.RunRepository {
			return workorchrepo.NewWorkspaceScopedFileRepositorySet(root).Runs
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			repo := factory(root)
			initial := &run.Run{ID: "run-001", WorkspaceID: "ws-integrity", State: common.RunStateActive, RuntimeBootstrap: &rc.RuntimeBootstrap{}, RuntimeState: &rc.RuntimeStateRecord{StateRevision: 1}}
			if err := repo.Create(ctx, initial); err != nil {
				t.Fatal(err)
			}
			original, err := repo.GetByWorkspaceID(ctx, initial.WorkspaceID, initial.ID)
			if err != nil {
				t.Fatal(err)
			}
			base, err := repo.GetRuntimeStateWriteBase(ctx, initial.WorkspaceID, initial.ID)
			if err != nil || base.Run == nil || base.RuntimeStateRevision != 1 || base.Run.RuntimeState != nil || base.Run.RuntimeLifecycleFence != nil || base.Run.Revision != original.Revision {
				t.Fatal("opaque write base contains old Runtime or loses physical revision", err)
			}
			base.Run.RuntimeBootstrap.Definition.Revision = "caller-mutated-metadata"
			unchanged, err := repo.GetByWorkspaceID(ctx, initial.WorkspaceID, initial.ID)
			if err != nil || !reflect.DeepEqual(unchanged, original) {
				t.Fatal("metadata read aliases repository", err)
			}
			if _, err := repo.GetRuntimeStateWriteBase(ctx, "foreign-workspace", initial.ID); !errors.Is(err, common.ErrNotFound) && !errors.Is(err, common.ErrConflict) {
				t.Fatal("write base crossed Workspace", err)
			}
			for _, kind := range []string{"wrong_fence", "changed_state", "unencodable"} {
				t.Run(kind, func(t *testing.T) {
					next := run.CloneRun(original)
					next.RuntimeState.StateRevision++
					next.RuntimeLifecycleFence = &rc.RuntimeOpaqueLifecycleFence{WorkspaceID: next.WorkspaceID, RunID: next.ID, RuntimeStateRevision: 2, RuntimeStateRef: "runtime-state:ws-integrity:run-001:2", RuntimeStateHash: rc.StableRuntimeStateRecordHash(*next.RuntimeState), BootstrapHash: rc.StableRuntimeBootstrapHash(*next.RuntimeBootstrap), TerminalFenceID: "terminal-test"}
					switch kind {
					case "wrong_fence":
						next.RuntimeLifecycleFence.RuntimeStateHash = "wrong"
					case "changed_state":
						next.RuntimeState.ProcessedEventIDs = []string{"original changed after fence"}
					case "unencodable":
						next.RuntimeState.Facts.Activation.Memories = []rc.RuntimeMemoryContext{{VectorScore: math.NaN()}}
					}
					if err := repo.CompareAndSwapRuntimeState(ctx, next, original.Revision, 1); !errors.Is(err, run.ErrRuntimeStateIntegrity) {
						t.Fatalf("expected integrity rejection: %v", err)
					}
					stored, err := repo.GetByWorkspaceID(ctx, initial.WorkspaceID, initial.ID)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(original, stored) {
						t.Fatal("rejected write changed stored record")
					}
				})
			}
			next := run.CloneRun(original)
			next.RuntimeState.StateRevision++
			if err := repo.CompareAndSwapRuntimeState(ctx, next, original.Revision, 1); err != nil {
				t.Fatal(err)
			}
			stored, err := factory(root).GetByWorkspaceID(ctx, initial.WorkspaceID, initial.ID)
			if name == "memory" {
				stored, err = repo.GetByWorkspaceID(ctx, initial.WorkspaceID, initial.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if stored.RuntimeStateHash == "" || stored.RuntimeStateHash != rc.StableRuntimeStateRecordHash(*next.RuntimeState) || stored.Revision != original.Revision+1 {
				t.Fatal("CAS did not persist exact new identity")
			}
			if err := repo.CompareAndSwapRuntimeState(ctx, run.CloneRun(original), original.Revision, 1); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("stale CAS: %v", err)
			}
			invalid := &run.Run{ID: "run-bad", WorkspaceID: "ws-bad", RuntimeState: &rc.RuntimeStateRecord{Facts: rc.RuntimeFacts{Activation: rc.RuntimeActivationContext{Memories: []rc.RuntimeMemoryContext{{VectorScore: math.Inf(1)}}}}}}
			if err := repo.Create(ctx, invalid); !errors.Is(err, run.ErrRuntimeStateIntegrity) {
				t.Fatalf("unencodable create: %v", err)
			}
			if _, err := repo.GetByWorkspaceID(ctx, invalid.WorkspaceID, invalid.ID); !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("invalid create published record: %v", err)
			}
		})
	}
}
