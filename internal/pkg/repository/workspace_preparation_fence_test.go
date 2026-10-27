package repository_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationFencePersistenceCASAndReopen(t *testing.T) {
	for _, backend := range []string{"memory", "file", "scoped"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			open := func() repository.WorkspaceRepository {
				switch backend {
				case "file":
					return file.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
				case "scoped":
					return workorchrepo.NewWorkspaceScopedFileRepositorySet(root).Workspaces
				default:
					return memory.NewWorkspaceRepository()
				}
			}
			repo := open()
			original := &workspace.Workspace{ID: "ws", PreparationFence: &workspace.WorkspacePreparationFence{
				Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: strings.Repeat("a", 32),
			}}
			if err := repo.Create(ctx, original); err != nil {
				t.Fatal(err)
			}
			original.PreparationFence.CreationEpoch = "caller-mutated"
			current, err := repo.Get(ctx, "ws")
			if err != nil || current.PreparationFence.CreationEpoch != strings.Repeat("a", 32) {
				t.Fatalf("create clone: %+v %v", current, err)
			}
			current.PreparationFence.PreparationEpoch = 1
			current.PreparationFence.Pending = &workspace.WorkspaceFreshIntentReservation{RequestID: "request", Kind: runtimecontract.RuntimeExecutionIntentRestart, StartupInputHash: "startup", PayloadHash: strings.Repeat("b", 64), RunPayloadHash: strings.Repeat("c", 64)}
			if err := repo.Update(ctx, current); err == nil {
				t.Fatal("ordinary update reserved without CAS")
			}
			if err := repo.CompareAndSwap(ctx, current, current.Revision+1); err == nil {
				t.Fatal("stale revision reserved")
			}
			if err := repo.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			expected := current.PreparationFence.Clone()
			current.PreparationFence.Pending.RequestID = "caller-mutated"
			if backend != "memory" {
				repo = open()
			}
			for _, operation := range []string{"get", "list", "update", "cas"} {
				read, err := repo.Get(ctx, "ws")
				if err != nil || !reflect.DeepEqual(read.PreparationFence, expected) {
					t.Fatalf("reopen/get: %+v %v", read, err)
				}
				if operation == "list" {
					items, err := repo.List(ctx)
					if err != nil || len(items) != 1 || !reflect.DeepEqual(items[0].PreparationFence, expected) {
						t.Fatalf("list: %+v %v", items, err)
					}
					read = items[0]
				}
				read.PreparationFence.Pending.PayloadHash = strings.Repeat("c", 64)
				if operation == "update" && repo.Update(ctx, read) == nil {
					t.Fatal("update replaced pending")
				}
				if operation == "cas" && repo.CompareAndSwap(ctx, read, read.Revision) == nil {
					t.Fatal("CAS replaced pending")
				}
			}
			for _, mutation := range []string{"erase", "clear", "epoch", "creation", "scope", "version"} {
				read, _ := repo.Get(ctx, "ws")
				switch mutation {
				case "erase":
					read.PreparationFence = nil
				case "clear":
					read.PreparationFence.Pending = nil
					read.PreparationFence.PreparationEpoch = 0
				case "epoch":
					read.PreparationFence.PreparationEpoch++
				case "creation":
					read.PreparationFence.CreationEpoch = strings.Repeat("d", 32)
				case "scope":
					read.PreparationFence.WorkspaceID = "other"
				case "version":
					read.PreparationFence.Version = "unknown"
				}
				if repo.Update(ctx, read) == nil || repo.CompareAndSwap(ctx, read, read.Revision) == nil {
					t.Fatalf("%s altered immutable pending fence", mutation)
				}
			}
			read, _ := repo.Get(ctx, "ws")
			read.Title = "metadata"
			if err := repo.CompareAndSwap(ctx, read, read.Revision); err != nil {
				t.Fatal(err)
			}
			read.Title = "ordinary metadata"
			if err := repo.Update(ctx, read); err != nil {
				t.Fatal(err)
			}
			read, _ = repo.Get(ctx, "ws")
			if !reflect.DeepEqual(read.PreparationFence, expected) {
				t.Fatal("metadata lost original fence")
			}
			legacy := &workspace.Workspace{ID: "legacy"}
			if err := repo.Create(ctx, legacy); err != nil {
				t.Fatal(err)
			}
			legacy.PreparationFence = expected.Clone()
			legacy.PreparationFence.WorkspaceID = legacy.ID
			legacy.PreparationFence.Pending = nil
			legacy.PreparationFence.PreparationEpoch = 0
			if repo.Update(ctx, legacy) == nil || repo.CompareAndSwap(ctx, legacy, legacy.Revision) == nil {
				t.Fatal("silently upgraded legacy")
			}
		})
	}
}

func TestPreparationFenceFileRawCorruptionFailsWithoutRepair(t *testing.T) {
	for _, corrupt := range []string{
		`{"Version":"unknown","WorkspaceID":"ws","CreationEpoch":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
		`{"Version":"workorch-preparation-fence/v1","WorkspaceID":"other","CreationEpoch":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
		`{"Version":"workorch-preparation-fence/v1","WorkspaceID":"ws","CreationEpoch":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`,
		`{"Version":"workorch-preparation-fence/v1","WorkspaceID":"ws","CreationEpoch":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","PreparationEpoch":1}`,
		`{"Version":"workorch-preparation-fence/v1","WorkspaceID":"ws","CreationEpoch":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Unknown":true}`,
	} {
		t.Run(corrupt, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "workspaces.json")
			before := []byte(`{"ws":{"ID":"ws","Revision":1,"PreparationFence":` + corrupt + `}}`)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			repo := file.NewWorkspaceRepository(path)
			if _, err := repo.Get(context.Background(), "ws"); err == nil {
				t.Fatal("corrupt raw fence accepted")
			}
			if _, err := repo.List(context.Background()); err == nil {
				t.Fatal("corrupt raw fence listed")
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("read repaired raw state")
			}
		})
	}
}
