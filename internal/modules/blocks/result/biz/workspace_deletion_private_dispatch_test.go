package result

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestWorkspaceDeletionPhysicalPurgeUsesOwnerPrivateDispatcher(t *testing.T) {
	calls := workspaceDeletionFunctionCalls(t, "workspace_deletion_internal_event.go")
	for _, forbidden := range []string{"WithWorkspacePurgeFence", "purgeWorkspaceData"} {
		if calls["purgeWorkspace"][forbidden] {
			t.Fatalf("command adapter purgeWorkspace directly calls physical sink %s", forbidden)
		}
	}
	for _, required := range []string{"WithWorkspacePurgeFence", "purgeWorkspaceData"} {
		if !calls["applyWorkspaceDeletionInternalEvent"][required] {
			t.Fatalf("owner-private dispatcher is missing physical sink %s", required)
		}
	}
}

func TestWorkspaceDeletionPrivateDispatcherRejectsForgedPurgeCompletion(t *testing.T) {
	ctx := context.Background()
	scope := deletioncontract.Scope{
		WorkspaceID: "ws-result-private-completion", DeletionID: "delete-result-private-completion",
		DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-result-private-completion",
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	current, err := reduceWorkspaceDeletionInternalEvent(nil, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope,
	}, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	repository := memory.NewResultWorkspaceDeletionRepository()
	if err := repository.Create(ctx, current); err != nil {
		t.Fatal(err)
	}
	biz := &Biz{workspaceDeletions: repository}
	if _, err := biz.applyWorkspaceDeletionInternalEvent(ctx, current, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: scope,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged purge_completed err = %v, want conflict", err)
	}
	stored, err := repository.Get(ctx, scope.WorkspaceID)
	if err != nil || stored.Phase != resultmodel.WorkspaceDeletionPhaseFenced || stored.Revision != 1 {
		t.Fatalf("forged purge_completed changed tombstone: stored=%+v err=%v", stored, err)
	}
}

func workspaceDeletionFunctionCalls(t *testing.T, path string) map[string]map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]map[string]bool{}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		called := map[string]bool{}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.Ident:
				called[target.Name] = true
			case *ast.SelectorExpr:
				called[target.Sel.Name] = true
			}
			return true
		})
		result[function.Name.Name] = called
	}
	return result
}
