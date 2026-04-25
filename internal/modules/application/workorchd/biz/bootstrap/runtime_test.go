package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
)

type testLifecycle struct {
	id          string
	runSeq      *[]string
	teardownSeq *[]string
	runErr      error
	teardownErr error
}

func (l testLifecycle) ID() string { return l.id }

func (l testLifecycle) Run() error {
	if l.runSeq != nil {
		*l.runSeq = append(*l.runSeq, l.id)
	}
	return l.runErr
}

func (l testLifecycle) Teardown(context.Context) error {
	if l.teardownSeq != nil {
		*l.teardownSeq = append(*l.teardownSeq, l.id)
	}
	return l.teardownErr
}

func TestNewWithBackendMemoryBuildsRuntime(t *testing.T) {
	rt := NewWithBackend("memory", "")
	if rt == nil {
		t.Fatal("expected runtime")
	}
	exports := rt.Exports()

	if err := exports.Services.WorkspaceService.CreateWorkspace(context.Background(), workspacemodule.CreateWorkspaceCommand{
		WorkspaceID: common.WorkspaceID("ws-bootstrap-memory"),
		Title:       "bootstrap memory backend",
	}); err != nil {
		t.Fatalf("create workspace with memory backend: %v", err)
	}

	got, err := exports.Repositories.Workspaces.Get(context.Background(), common.WorkspaceID("ws-bootstrap-memory"))
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
	exports := rt.Exports()

	err := exports.Services.WorkspaceService.CreateWorkspace(context.Background(), workspacemodule.CreateWorkspaceCommand{
		WorkspaceID: common.WorkspaceID("ws-bootstrap-file"),
		Title:       "bootstrap file backend",
	})
	if err != nil {
		t.Fatalf("create workspace with file backend: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "workspace.json")); err != nil {
		t.Fatalf("expected workspace persistence file: %v", err)
	}
}

func TestNewWithBackendFileUsesDefaultDataRootWhenUnset(t *testing.T) {
	tempRoot := t.TempDir()
	defaultDataDir := filepath.Join(tempRoot, "workorchd-data")
	t.Setenv("TMPDIR", tempRoot)

	rt := NewWithBackend("file", "")
	exports := rt.Exports()

	err := exports.Services.WorkspaceService.CreateWorkspace(context.Background(), workspacemodule.CreateWorkspaceCommand{
		WorkspaceID: common.WorkspaceID("ws-bootstrap-file-default"),
		Title:       "bootstrap file backend default data root",
	})
	if err != nil {
		t.Fatalf("create workspace with default file backend: %v", err)
	}

	if _, err := os.Stat(filepath.Join(defaultDataDir, "workspace.json")); err != nil {
		t.Fatalf("expected workspace persistence file in default data dir: %v", err)
	}
}

func TestModulePluginLifecycleRunsAndShutsDownInOrder(t *testing.T) {
	var runSeq []string
	var teardownSeq []string
	lifecycle, err := newModulePluginLifecycle([]lifecycle{
		testLifecycle{id: "first", runSeq: &runSeq, teardownSeq: &teardownSeq},
		testLifecycle{id: "second", runSeq: &runSeq, teardownSeq: &teardownSeq},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}

	if err := lifecycle.Run(); err != nil {
		t.Fatalf("run lifecycle: %v", err)
	}
	if got, want := runSeq, []string{"first", "second"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("run seq = %v, want %v", got, want)
	}

	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown lifecycle: %v", err)
	}
	if got, want := teardownSeq, []string{"second", "first"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("teardown seq = %v, want %v", got, want)
	}
}

func TestModulePluginLifecycleReturnsRunAndShutdownErrors(t *testing.T) {
	errRun := errors.New("run failed")
	pluginLifecycle, err := newModulePluginLifecycle([]lifecycle{
		testLifecycle{id: "broken-run", runErr: errRun},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	if err := pluginLifecycle.Run(); err == nil || !strings.Contains(err.Error(), errRun.Error()) {
		t.Fatalf("run err = %v, want %v", err, errRun)
	}

	errShutdown := errors.New("shutdown failed")
	pluginLifecycle, err = newModulePluginLifecycle([]lifecycle{
		testLifecycle{id: "broken-shutdown", teardownErr: errShutdown},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	if err := pluginLifecycle.Shutdown(context.Background()); !errors.Is(err, errShutdown) {
		t.Fatalf("shutdown err = %v, want %v", err, errShutdown)
	}
}
