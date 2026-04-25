package appservice

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCreateWorkspaceWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := CreateWorkspaceWithSession(context.Background(), nil, "ws-1", "workspace 1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestCreateWorkspaceWithBackendReturnsCreatedWorkspace(t *testing.T) {
	item, err := CreateWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "ws-appservice-created", "appservice created workspace")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if item.ID != "ws-appservice-created" || item.Title != "appservice created workspace" {
		t.Fatalf("workspace = %+v, want created workspace", item)
	}
	if item.State != "active" {
		t.Fatalf("state = %q, want active", item.State)
	}
}

func TestReadWorkspaceWithBackendReturnsPersistedWorkspace(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-appservice-read", "appservice read workspace"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	item, err := ReadWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-appservice-read")
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if item.ID != "ws-appservice-read" || item.Title != "appservice read workspace" {
		t.Fatalf("workspace = %+v, want persisted workspace", item)
	}
}

func TestReadWorkspaceWithBackendReturnsNotFoundWhenMissing(t *testing.T) {
	if _, err := ReadWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "ws-missing"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestListWorkspacesWithBackendReturnsSortedWorkspaces(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-b", "workspace b"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-b: %v", err)
	}
	if _, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "ws-a", "workspace a"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-a: %v", err)
	}

	items, err := ListWorkspacesWithBackend(context.Background(), "file", dataRoot)
	if err != nil {
		t.Fatalf("ListWorkspacesWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "ws-a" || items[1].ID != "ws-b" {
		t.Fatalf("workspace ids = %q/%q, want ws-a/ws-b", items[0].ID, items[1].ID)
	}
}
