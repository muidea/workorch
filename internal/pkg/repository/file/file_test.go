package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
