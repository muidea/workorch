package repository_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepository "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestWorkspaceDeletionMaterialCleanerPreservesScopeErrorCategory(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		cleaner repository.WorkspaceDeletionMaterialCleaner
	}{
		{name: "memory", cleaner: memoryrepository.NewWorkspaceRepository()},
		{name: "file", cleaner: filerepository.NewWorkspaceRepository(filepath.Join(t.TempDir(), "workspaces.json"))},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			scope := repository.WorkspaceDeletionMaterialCleanupScope{
				WorkspaceID: "ws-cleaner-errors", DeletionID: "delete-cleaner-errors", DeletionGeneration: 1,
				IntentHash: "intent-cleaner-errors", ExpectedWorkspaceRevision: 1,
				ExpectedDeletionRevision: 1, CleanupID: "cleanup-cleaner-errors",
			}
			if err := testCase.cleaner.CleanupDeletionMaterial(context.Background(), scope); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("unbound context error = %v, want conflict", err)
			}
			bound, err := common.BindRuntimeWorkspaceContext(context.Background(), scope.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			invalid := scope
			invalid.CleanupID = ""
			if err := testCase.cleaner.CleanupDeletionMaterial(bound, invalid); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("invalid scope error = %v, want invalid argument", err)
			}
			if err := testCase.cleaner.CleanupDeletionMaterial(bound, scope); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("bound missing aggregate error = %v, want conflict", err)
			}
		})
	}
}
