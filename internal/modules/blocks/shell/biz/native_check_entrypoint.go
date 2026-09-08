package shell

import (
	"go/ast"
	"go/token"
	"strings"
)

// v4 re-observes existing input bytes under an explicitly selected contract;
// it does not rewrite them, migrate a v3 binding or infer assertion semantics.
// Reachable testing.T assertions still use the shared bounded native walker.
func shellEntrypointSupplementDeclarations(file *ast.File, native string) bool {
	for _, decl := range file.Decls {
		switch item := decl.(type) {
		case *ast.GenDecl:
			switch item.Tok {
			case token.IMPORT, token.TYPE, token.CONST:
				continue
			case token.VAR:
				pure := true
				ast.Inspect(item, func(n ast.Node) bool {
					switch n.(type) {
					case *ast.CallExpr, *ast.FuncLit:
						pure = false
					}
					return pure
				})
				if pure {
					continue
				}
			}
		case *ast.FuncDecl:
			if item.Recv == nil && item.Name.Name != "init" && item.Name.Name != "TestMain" &&
				(!strings.HasPrefix(item.Name.Name, "Test") || item.Name.Name == native) {
				continue
			}
		}
		return false
	}
	return true
}

func shellNativeRunnerOverride(file *ast.File) bool {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && (fn.Name.Name == "init" || fn.Name.Name == "TestMain") {
			return true
		}
	}
	return false
}
