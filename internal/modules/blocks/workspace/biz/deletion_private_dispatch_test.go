package workspace

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestWorkspaceOwnedPhysicalCleanupUsesPrivateDispatcher(t *testing.T) {
	adapterCalls := workspaceDeletionPrivateFunctionCalls(t, "deletion_coordinator.go", "driveWorkspaceDeletionDirectoryCleanup")
	if adapterCalls["cleanupWorkspaceOwnedDeletionMaterial"] {
		t.Fatal("Workspace deletion coordinator directly calls physical cleanup sink")
	}
	dispatcherCalls := workspaceDeletionPrivateFunctionCalls(t, "deletion_internal_event.go", "submitWorkspaceDeletionInternalEvent")
	if !dispatcherCalls["cleanupWorkspaceOwnedDeletionMaterial"] {
		t.Fatal("Workspace owner-private deletion dispatcher is missing physical cleanup sink")
	}
	if !dispatcherCalls["CleanupDeletionMaterial"] {
		t.Fatal("Workspace owner-private deletion dispatcher is missing exact sidecar/index cleanup sink")
	}
}

func TestWorkspaceDeletionMaterialCleanerHasOneProductionCaller(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "..", "..", ".."))
	wantPath := filepath.ToSlash(filepath.Join("internal", "modules", "blocks", "workspace", "biz", "deletion_internal_event.go"))
	calls := 0
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "CleanupDeletionMaterial" {
					return true
				}
				calls++
				relative, _ := filepath.Rel(root, path)
				if filepath.ToSlash(relative) != wantPath || function.Name.Name != "submitWorkspaceDeletionInternalEvent" {
					t.Errorf("CleanupDeletionMaterial production caller = %s:%s, want Workspace private dispatcher", filepath.ToSlash(relative), function.Name.Name)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("CleanupDeletionMaterial production call count = %d, want 1", calls)
	}
}

func TestWorkspaceDeletionPrivateDispatcherRejectsForgedDirectoryCleanupCompletion(t *testing.T) {
	biz := &Biz{workspaces: memoryrepo.NewWorkspaceRepository()}
	_, err := biz.submitWorkspaceDeletionInternalEvent(context.Background(), workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventDirectoryCleanupObserved,
		DirectoryCleanup: &workspaceDeletionDirectoryCleanupFact{
			WorkspaceID: "ws-forged-cleanup", DeletionID: "delete-forged-cleanup",
			CleanupID: "cleanup-forged", OccurredAt: time.Now().UTC(),
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged directory_cleanup_observed err = %v, want conflict", err)
	}
}

func workspaceDeletionPrivateFunctionCalls(t *testing.T, path, name string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != name || function.Body == nil {
			continue
		}
		calls := map[string]bool{}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.Ident:
				calls[target.Name] = true
			case *ast.SelectorExpr:
				calls[target.Sel.Name] = true
			}
			return true
		})
		return calls
	}
	t.Fatalf("function %s was not found in %s", name, path)
	return nil
}
