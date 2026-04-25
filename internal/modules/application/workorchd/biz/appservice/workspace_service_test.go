package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCreateWorkspaceWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := CreateWorkspaceWithSession(context.Background(), nil, "workspace 1", "", "workspaces/workspace 1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestCreateWorkspaceWithBackendReturnsCreatedWorkspace(t *testing.T) {
	item, err := CreateWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "appservice created workspace", "appservice description", "workspaces/appservice created workspace")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if item.ID == "" || item.Title != "appservice created workspace" || item.Description != "appservice description" || item.WorkingDirectory != "workspaces/appservice created workspace" {
		t.Fatalf("workspace = %+v, want created workspace", item)
	}
	if !strings.HasPrefix(item.ID, "ws-") {
		t.Fatalf("workspace id = %q, want generated ws- id", item.ID)
	}
	if item.State != "active" {
		t.Fatalf("state = %q, want active", item.State)
	}
}

func TestReadWorkspaceWithBackendReturnsPersistedWorkspace(t *testing.T) {
	dataRoot := t.TempDir()
	created, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "appservice read workspace", "read description", "workspaces/appservice read workspace")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	item, err := ReadWorkspaceWithBackend(context.Background(), "file", dataRoot, created.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if item.ID != created.ID || item.Title != "appservice read workspace" || item.Description != "read description" || item.WorkingDirectory != "workspaces/appservice read workspace" {
		t.Fatalf("workspace = %+v, want persisted workspace", item)
	}
}

func TestCreateWorkspaceWithBackendDefaultsWorkingDirectory(t *testing.T) {
	item, err := CreateWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "appservice default directory workspace", "", "")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	want := filepath.Join(wd, "workspaces", item.ID)
	if item.WorkingDirectory != want {
		t.Fatalf("workingDirectory = %q, want %q", item.WorkingDirectory, want)
	}
}

func TestReadWorkspaceWithBackendReturnsNotFoundWhenMissing(t *testing.T) {
	if _, err := ReadWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "ws-missing"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestListWorkspacesWithBackendReturnsSortedWorkspaces(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-b", "workspace b", "", "workspaces/workspace b"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-b: %v", err)
	}
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-a", "workspace a", "", "workspaces/workspace a"); err != nil {
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
