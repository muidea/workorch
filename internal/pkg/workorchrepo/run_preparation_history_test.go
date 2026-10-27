package workorchrepo

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestPreparationHistoryScopedInventoryRejectsPartialCorruptAndSymlinkedStorageWithoutWrites(t *testing.T) {
	for _, scenario := range []string{"empty_directory", "empty_file_without_head", "corrupt_run", "missing_head", "corrupt_head", "non_directory", "run_symlink", "runs_symlink", "head_symlink"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			r := NewWorkspaceScopedFileRepositorySet(root).Runs.(*workspaceScopedRunRepository)
			ws := common.WorkspaceID("history")
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), ws)
			runsRoot := filepath.Join(r.store.workspaceRoot(ws), "runs")
			runRoot := filepath.Join(runsRoot, "r")
			write := func(path string, data []byte) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			link := func(target, path string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "empty_directory":
				if err := os.MkdirAll(runRoot, 0o700); err != nil {
					t.Fatal(err)
				}
			case "empty_file_without_head":
				write(filepath.Join(runRoot, workspaceScopedRunFile), []byte(`{}`))
			case "corrupt_run":
				write(filepath.Join(runRoot, workspaceScopedRunFile), []byte(`{"r":`))
			case "missing_head":
				data, err := json.Marshal(map[string]runmodel.Run{"r": {ID: "r", WorkspaceID: ws, Revision: 1, WorkspaceRunGeneration: 1, State: common.RunStateCompleted}})
				if err != nil {
					t.Fatal(err)
				}
				write(filepath.Join(runRoot, workspaceScopedRunFile), data)
			case "corrupt_head":
				write(r.store.workspaceRunGenerationPath(ws), []byte(`{"generation":`))
			case "non_directory":
				write(filepath.Join(runsRoot, "partial"), []byte(`{}`))
			case "run_symlink":
				link(filepath.Join(root, "missing"), runRoot)
			case "runs_symlink":
				link(t.TempDir(), runsRoot)
			case "head_symlink":
				link(filepath.Join(root, "missing"), r.store.workspaceRunGenerationPath(ws))
			}
			before := preparationHistoryTreeSnapshot(t, root)
			value, err := r.ObserveWorkspacePreparationHistory(ctx, ws)
			if err == nil || value.WorkspaceID != "" {
				t.Fatalf("partial storage was an empty complete history: %+v %v", value, err)
			}
			if !reflect.DeepEqual(before, preparationHistoryTreeSnapshot(t, root)) {
				t.Fatal("read repaired or changed physical owner state")
			}
		})
	}
}

func TestPreparationHistoryScopedEmptyReadDoesNotCreateDirectories(t *testing.T) {
	root := t.TempDir()
	r := NewWorkspaceScopedFileRepositorySet(root).Runs.(*workspaceScopedRunRepository)
	before := preparationHistoryTreeSnapshot(t, root)
	value, err := r.ObserveWorkspacePreparationHistory(context.Background(), "new-workspace")
	if err != nil || value.GenerationHead != 0 || value.RunCount != 0 {
		t.Fatalf("fresh complete read: %+v %v", value, err)
	}
	if !reflect.DeepEqual(before, preparationHistoryTreeSnapshot(t, root)) {
		t.Fatal("empty read created owner files/directories")
	}
}

func preparationHistoryTreeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	values := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		values[path] = info.Mode().String()
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			values[path] += string(data)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			values[path] += target
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return values
}
