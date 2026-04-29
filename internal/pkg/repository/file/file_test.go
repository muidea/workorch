package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceRepositoryGetReturnsErrorWhenPersistedJSONCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	corrupt := []byte("{not-json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt workspace file: %v", err)
	}

	repo := NewWorkspaceRepository(path)
	if _, err := repo.Get(context.Background(), "ws-corrupt"); err == nil {
		t.Fatal("Get err = nil, want json error")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workspace file after corrupt load: %v", err)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("workspace file changed after corrupt load: got %q want %q", string(after), string(corrupt))
	}
}

func TestRunRepositoryCreateReturnsErrorWhenPersistedJSONCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.json")
	corrupt := []byte("{not-json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt run file: %v", err)
	}

	repo := NewRunRepository(path)
	err := repo.Create(context.Background(), &runmodel.Run{
		ID:          common.RunID("run-corrupt"),
		WorkspaceID: common.WorkspaceID("ws-corrupt"),
		State:       common.RunStateCreated,
	})
	if err == nil {
		t.Fatal("Create err = nil, want json error")
	}
	if errors.Is(err, common.ErrConflict) || errors.Is(err, common.ErrNotFound) {
		t.Fatalf("Create err = %v, want persisted json error", err)
	}

	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read run file after corrupt create: %v", readErr)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("run file changed after corrupt create: got %q want %q", string(after), string(corrupt))
	}
}

func TestWorkspaceRepositoryCreateStillSucceedsAfterEmptyPersistedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty workspace file: %v", err)
	}

	repo := NewWorkspaceRepository(path)
	if err := repo.Create(context.Background(), &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-empty"),
		Title: "empty workspace",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("Create with empty persisted file: %v", err)
	}

	item, err := repo.Get(context.Background(), "ws-empty")
	if err != nil {
		t.Fatalf("Get after create with empty persisted file: %v", err)
	}
	if item.ID != "ws-empty" || item.Title != "empty workspace" || item.State != common.WorkspaceStateActive {
		t.Fatalf("workspace = %+v, want ws-empty/empty workspace/active", item)
	}
}

func TestWorkspaceRepositorySharedInstanceConcurrentCreate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.json")
	repo := NewWorkspaceRepository(path)
	const total = 100

	var wg sync.WaitGroup
	errs := make(chan error, total)
	for idx := 0; idx < total; idx++ {
		idx := idx
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repo.Create(ctx, &workspacemodel.Workspace{
				ID:    common.WorkspaceID(fmt.Sprintf("ws-shared-%03d", idx)),
				Title: fmt.Sprintf("shared workspace %03d", idx),
				State: common.WorkspaceStateActive,
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create with shared repository instance: %v", err)
		}
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List after concurrent Create: %v", err)
	}
	if len(items) != total {
		t.Fatalf("workspace count = %d, want %d", len(items), total)
	}
}

func TestWorkspaceRepositoryIndependentInstancesConcurrentCreateStress(t *testing.T) {
	if os.Getenv("WORKORCH_STRESS_TEST") != "1" {
		t.Skip("set WORKORCH_STRESS_TEST=1 to run file backend cross-instance concurrency stress")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.json")
	const total = 100

	var wg sync.WaitGroup
	errs := make(chan error, total)
	for idx := 0; idx < total; idx++ {
		idx := idx
		wg.Add(1)
		go func() {
			defer wg.Done()
			repo := NewWorkspaceRepository(path)
			errs <- repo.Create(ctx, &workspacemodel.Workspace{
				ID:    common.WorkspaceID(fmt.Sprintf("ws-independent-%03d", idx)),
				Title: fmt.Sprintf("independent workspace %03d", idx),
				State: common.WorkspaceStateActive,
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create with independent repository instances: %v", err)
		}
	}

	items, err := NewWorkspaceRepository(path).List(ctx)
	if err != nil {
		t.Fatalf("List after independent concurrent Create: %v", err)
	}
	if len(items) != total {
		t.Fatalf("workspace count = %d, want %d", len(items), total)
	}
}
