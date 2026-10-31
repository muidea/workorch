package repository_test

import (
	"context"
	"path/filepath"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestPreparationDefinitionCompilationCannotBeImportedWithoutOriginalSource(t *testing.T) {
	for _, backend := range []string{"memory", "file"} {
		t.Run(backend, func(t *testing.T) {
			var repo repository.WorkspaceRepository = memory.NewWorkspaceRepository()
			if backend == "file" {
				repo = file.NewWorkspaceRepository(filepath.Join(t.TempDir(), "workspaces.json"))
			}
			w := &workspace.Workspace{ID: "unbound-compilation", Revision: 1, StartupPreparation: &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationWorkflowVersion, WorkspaceID: "unbound-compilation", Attempts: []workspace.WorkspacePreparationAttempt{{DefinitionCompilation: &workspace.PreparationDefinitionCompilation{CompilerVersion: workspace.PreparationDefinitionCompilerVersion, SourceInventoryHash: "caller-hash"}}}}}
			if repo.Create(context.Background(), w) == nil {
				t.Fatal("imported compilation without original seal")
			}
			if _, err := repo.Get(context.Background(), w.ID); err == nil {
				t.Fatal("rejected import persisted")
			}
		})
	}
}
