package repository_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionEventRecordRepositoriesExposeNoGenericWorkspacePurge(t *testing.T) {
	t.Helper()
	err := filepath.WalkDir("../..", func(path string, entry fs.DirEntry, walkErr error) error {
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
			if !ok || function.Recv == nil || function.Name.Name != "PurgeWorkspace" {
				continue
			}
			receiver := ""
			if len(function.Recv.List) == 1 {
				receiver = receiverTypeName(function.Recv.List[0].Type)
			}
			if strings.Contains(receiver, "EventRecordRepository") {
				t.Errorf("forbidden generic EventRecord workspace purge: %s receiver=%s", path, receiver)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func receiverTypeName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverTypeName(value.X)
	case *ast.IndexExpr:
		return receiverTypeName(value.X)
	case *ast.IndexListExpr:
		return receiverTypeName(value.X)
	default:
		return ""
	}
}
