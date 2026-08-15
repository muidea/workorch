package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// TestO6GovernanceAdaptersUseFocusedMappings freezes the protocol side of the
// O6 split. EventHub adapters may load a scoped aggregate and map concrete
// commands/results, but ticket/dispatch replay lookup belongs to the stateless
// governancecheck package and mutations still enter through the private event.
func TestO6GovernanceAdaptersUseFocusedMappings(t *testing.T) {
	t.Parallel()
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", func(info os.FileInfo) bool {
		return filepath.Ext(info.Name()) == ".go" && len(info.Name()) > len("_test.go") && info.Name()[len(info.Name())-len("_test.go"):] != "_test.go"
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := packages["orchestrator"]
	if pkg == nil {
		t.Fatal("orchestrator package is missing")
	}
	wantedFiles := map[string]bool{
		"runtime_governance_event_adapter.go":   true,
		"runtime_governance_request_adapter.go": true,
		"runtime_drive.go":                      true,
	}
	expectedCalls := map[string]int{
		"RequestForTicket":   1,
		"RequestForDispatch": 2,
	}
	actualCalls := map[string]int{}
	parsedFiles := 0
	for filename, file := range pkg.Files {
		if !wantedFiles[filepath.Base(filename)] {
			continue
		}
		parsedFiles++
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CallExpr:
				selector, ok := value.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				qualifier, ok := selector.X.(*ast.Ident)
				if ok && qualifier.Name == "governancecheck" {
					actualCalls[selector.Sel.Name]++
				}
				if selector.Sel.Name == "ApplyInternalEvent" {
					t.Fatalf("governance adapter %s bypasses Biz.submitRuntimeInternalEvent", filepath.Base(filename))
				}
			case *ast.RangeStmt:
				selector, ok := value.X.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "GovernanceRequests" {
					t.Fatalf("governance adapter %s reintroduced an inline GovernanceRequests scan", filepath.Base(filename))
				}
			}
			return true
		})
	}
	if parsedFiles != len(wantedFiles) {
		t.Fatalf("parsed governance adapter files = %d, want %d", parsedFiles, len(wantedFiles))
	}
	for name, count := range expectedCalls {
		if actualCalls[name] != count {
			t.Fatalf("governancecheck.%s adapter call sites = %d, want %d", name, actualCalls[name], count)
		}
	}
}
