package workorchruntimecontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// If the original gate starts reading a new source, its exact raw projection
// must be extended in the same change. Passing the whole source to a new helper
// is not a way to hide an untracked dependency from this footprint guard.
func TestLineageOperationSessionSourceProjectionCoversGateFootprint(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "runtime_lineage_assertion_consumer.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"Facts.WorkspaceID": true, "Facts.RunID": true, "Facts.Evidence": true,
		"Active": true, "Completed": true, "Attempts": true, "Sessions": true,
		"Governance": true, "Resolutions": true, "Receipts": true,
	}
	var body *ast.BlockStmt
	for _, declaration := range file.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Name.Name == "runtimeValidateLineageOperationsWithCompilers" {
			body = fn.Body
		}
	}
	if body == nil {
		t.Fatal("complete operation gate missing")
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok {
			var parts []string
			var root ast.Expr = selector
			for {
				member, ok := root.(*ast.SelectorExpr)
				if !ok {
					break
				}
				parts = append([]string{member.Sel.Name}, parts...)
				root = member.X
			}
			if identifier, ok := root.(*ast.Ident); ok && identifier.Name == "sources" {
				path := strings.Join(parts, ".")
				covered := false
				for field := range allowed {
					covered = covered || path == field || strings.HasPrefix(path, field+".")
				}
				if !covered {
					t.Errorf("complete operation gate reads uncaptured raw source %q", path)
				}
				return false
			}
		}
		if identifier, ok := node.(*ast.Ident); ok && identifier.Name == "sources" {
			t.Error("complete operation gate passes an unbounded source dependency")
		}
		return true
	})
}
