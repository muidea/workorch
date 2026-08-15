package shell

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRuntimeExecuteOperationKeepsSinglePhysicalProcessPath(t *testing.T) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve structure test source")
	}
	path := filepath.Join(filepath.Dir(currentFile), "shell_runtime_execute.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch selector.Sel.Name {
		case "CommandContext", "Start", "Wait":
			counts[selector.Sel.Name]++
		}
		return true
	})
	for _, name := range []string{"CommandContext", "Start", "Wait"} {
		if counts[name] != 1 {
			t.Fatalf("%s calls = %d, want one physical process path", name, counts[name])
		}
	}
}
