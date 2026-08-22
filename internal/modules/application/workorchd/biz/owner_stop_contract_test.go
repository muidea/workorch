package biz

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestApplicationStopPreservesTransportProjectionRuntimeOrder(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fset := token.NewFileSet()
	unit, err := parser.ParseFile(fset, filepath.Join(filepath.Dir(currentFile), "owner.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse owner.go: %v", err)
	}
	var stop *ast.FuncDecl
	for _, declaration := range unit.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "Stop" {
			stop = function
			break
		}
	}
	if stop == nil || stop.Body == nil {
		t.Fatal("Biz.Stop declaration not found")
	}
	wanted := []string{
		"admissions.stop",
		"b.cliDispatcher.Close",
		"b.service.DrainHTTP",
		"admissions.wait",
		"b.service.CloseTransportState",
		"b.service.SetWorkspaceExecutionAdmission",
		"b.service.SetWorkspaceProjection",
		"b.service.SetWorkspaceProjectionWriter",
		"b.UnsubscribeFunc",
		"b.projection.CloseContext",
		"b.runtime.close",
	}
	positions := make(map[string]token.Pos, len(wanted))
	ast.Inspect(stop.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := applicationSelectorName(call.Fun)
		for _, candidate := range wanted {
			if name == candidate {
				positions[candidate] = call.Pos()
			}
		}
		return true
	})
	for index, name := range wanted {
		if positions[name] == token.NoPos {
			t.Fatalf("Biz.Stop does not call %s", name)
		}
		if index > 0 && positions[wanted[index-1]] >= positions[name] {
			t.Fatalf("Biz.Stop order drift: %s must precede %s", wanted[index-1], name)
		}
	}
}

func applicationSelectorName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := applicationSelectorName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	default:
		return ""
	}
}
