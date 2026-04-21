package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestNewWithBackendMemoryBuildsRuntime(t *testing.T) {
	rt := NewWithBackend("memory", "")
	if rt == nil {
		t.Fatal("expected runtime")
	}

	if err := rt.CreateWorkspace(
		context.Background(),
		common.WorkspaceID("ws-bootstrap-memory"),
		"bootstrap memory backend",
	); err != nil {
		t.Fatalf("create workspace with memory backend: %v", err)
	}

	got, err := rt.GetWorkspaceByID(context.Background(), common.WorkspaceID("ws-bootstrap-memory"))
	if err != nil {
		t.Fatalf("get workspace with memory backend: %v", err)
	}
	if got.Title != "bootstrap memory backend" {
		t.Fatalf("workspace title mismatch: %s", got.Title)
	}
}

func TestNewWithBackendFilePersistsWorkspaceData(t *testing.T) {
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)

	err := rt.CreateWorkspace(
		context.Background(),
		common.WorkspaceID("ws-bootstrap-file"),
		"bootstrap file backend",
	)
	if err != nil {
		t.Fatalf("create workspace with file backend: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "workspace.json")); err != nil {
		t.Fatalf("expected workspace persistence file: %v", err)
	}
}
