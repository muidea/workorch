package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestTerminalDeliveryConsumesFormalOwnerResult(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "runtime_result_projection.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "deliverRuntimeTerminal" {
			continue
		}
		found = true
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Validate" {
				if id, ok := selector.X.(*ast.Ident); ok && id.Name == "aggregate" {
					t.Fatal("delivery re-admits the full historical owner result")
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("terminal consumption guard lost the production entry")
	}
}
