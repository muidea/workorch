package workorchrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func creationPersistenceFixture(request string) *workspacemodel.Workspace {
	return &workspacemodel.Workspace{
		ID: workspacemodel.DefinitionAuthoringWorkspaceID(request), Title: "原始草稿", State: common.WorkspaceStateActive,
		DeliverableDirectory: "project", InternalDirectory: "internal", DefaultSoulID: "soul-default",
		SoulBindings:                map[string]common.SoulID{"implementation": "soul-implementation"},
		DefinitionAuthoringCreation: &workspacemodel.DefinitionAuthoringCreation{RequestID: request, InputHash: "creation-input-hash"},
	}
}

func TestAuthoringCreationPersistenceRecoversRealFileCutpoints(t *testing.T) {
	for _, stage := range []string{"bindings", "index"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			first := NewWorkspaceScopedFileRepositorySet(root)
			repo := first.Workspaces.(*workspaceScopedWorkspaceRepository)
			item := creationPersistenceFixture(stage)
			obstacle := filepath.Join(repo.store.workspaceRoot(item.ID), workspaceScopedConfigDir)
			if stage == "index" {
				obstacle = filepath.Join(root, "system")
			}
			if err := os.MkdirAll(filepath.Dir(obstacle), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(obstacle, []byte("仅测试故障障碍"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := first.Workspaces.Create(ctx, item); err == nil {
				t.Fatal("partial publication reported success")
			}
			path := filepath.Join(repo.store.workspaceRoot(item.ID), workspaceScopedWorkspaceFile)
			metadata, err := repo.store.workspaceRepo(path).Get(ctx, item.ID)
			if err != nil || metadata.DefinitionAuthoringCreation.PendingPersistence == nil || metadata.Revision != 1 {
				t.Fatalf("missing durable after-image: %+v %v", metadata, err)
			}
			if err := first.Workspaces.Update(ctx, item); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("update crossed pending creation: %v", err)
			}
			if err := first.Workspaces.CompareAndSwap(ctx, item, 1); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("CAS crossed pending creation: %v", err)
			}
			if err := first.Workspaces.Delete(ctx, item.ID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("delete crossed pending creation: %v", err)
			}
			// 故障仍在时读取不能把缺少绑定／索引的 metadata 当作完整对象。
			if _, err := NewWorkspaceScopedFileRepositorySet(root).Workspaces.Get(ctx, item.ID); err == nil {
				t.Fatal("partial object became visible")
			}
			if err := os.Remove(obstacle); err != nil {
				t.Fatal(err)
			}
			// 列表首次恢复不能依赖此前按 ID Get 已经补齐索引。
			itemsBeforeGet, err := NewWorkspaceScopedFileRepositorySet(root).Workspaces.List(ctx)
			if err != nil || len(itemsBeforeGet) != 1 || itemsBeforeGet[0].ID != item.ID {
				t.Fatalf("list failed to recover missing index: %+v %v", itemsBeforeGet, err)
			}
			var wg sync.WaitGroup
			for range 4 {
				wg.Go(func() {
					got, err := NewWorkspaceScopedFileRepositorySet(root).Workspaces.Get(ctx, item.ID)
					if err != nil {
						t.Errorf("reopen recovery: %v", err)
						return
					}
					if got.Revision != 2 || got.DefaultSoulID != item.DefaultSoulID || got.SoulBindings["implementation"] != item.SoulBindings["implementation"] || got.DefinitionAuthoringCreation.PendingPersistence != nil {
						t.Errorf("incomplete recovery: %+v", got)
					}
				})
			}
			wg.Wait()
			reopened := NewWorkspaceScopedFileRepositorySet(root)
			items, err := reopened.Workspaces.List(ctx)
			if err != nil || len(items) != 1 || items[0].ID != item.ID {
				t.Fatalf("undiscoverable workspace: %+v %v", items, err)
			}
			got := items[0]
			got.Title, got.DefaultSoulID = "后续修订", "soul-new"
			if err := reopened.Workspaces.CompareAndSwap(ctx, got, got.Revision); err != nil {
				t.Fatal(err)
			}
			replay, err := NewWorkspaceScopedFileRepositorySet(root).Workspaces.Get(ctx, item.ID)
			if err != nil || replay.Title != got.Title || replay.DefaultSoulID != got.DefaultSoulID || replay.Revision != got.Revision {
				t.Fatalf("replay overwrote later revision: %+v %v", replay, err)
			}
		})
	}
}

func TestAuthoringCreationPersistenceRefusesConflictingAfterImages(t *testing.T) {
	for _, mutation := range []string{"binding", "index", "revision", "deletion"} {
		t.Run(mutation, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			repo := NewWorkspaceScopedFileRepositorySet(root).Workspaces.(*workspaceScopedWorkspaceRepository)
			item := creationPersistenceFixture(mutation)
			metadata := workspaceMetadata(item)
			identity := item.DefinitionAuthoringCreation.Clone()
			identity.PendingPersistence = &workspacemodel.DefinitionAuthoringCreationPersistence{WorkspaceRevision: 1, DefaultSoulID: item.DefaultSoulID, SoulBindings: item.SoulBindings}
			metadata.DefinitionAuthoringCreation = &identity
			if mutation == "revision" {
				metadata.Revision = 3
			}
			if mutation == "deletion" {
				metadata.Deletion = &workspacemodel.WorkspaceDeletionRecord{}
			}
			path := filepath.Join(repo.store.workspaceRoot(item.ID), workspaceScopedWorkspaceFile)
			if err := repo.store.workspaceRepo(path).Create(ctx, metadata); err != nil {
				t.Fatal(err)
			}
			if mutation == "binding" {
				changed := *item
				changed.DefaultSoulID = "external-soul"
				if err := repo.store.saveWorkspaceSoulBindings(&changed); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "index" {
				changed := *item
				changed.DeliverableDirectory = "external-project"
				if err := repo.store.upsertWorkspaceIndex(&changed); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := NewWorkspaceScopedFileRepositorySet(root).Workspaces.Get(ctx, item.ID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("conflict accepted: %v", err)
			}
			current, err := repo.store.workspaceRepo(path).Get(ctx, item.ID)
			if err != nil || current.Revision != metadata.Revision || current.DefinitionAuthoringCreation.PendingPersistence == nil {
				t.Fatalf("conflict erased journal: %+v %v", current, err)
			}
			if mutation == "index" {
				entries, err := repo.store.listWorkspaceIndexEntries()
				if err != nil || len(entries) != 1 || entries[0].DeliverableDirectory != "external-project" {
					t.Fatal("recovery overwrote conflicting index")
				}
			}
			if mutation == "binding" {
				bindings, err := repo.store.loadWorkspaceSoulBindings(item.ID)
				if err != nil || bindings.DefaultSoulID != "external-soul" {
					t.Fatal("recovery overwrote conflicting binding")
				}
			}
		})
	}
}
