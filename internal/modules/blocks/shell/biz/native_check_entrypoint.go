package shell

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// A native test may execute delivery binaries. Re-entering its own test binary
// must go through the owner channel: shared stdout has no recoverable native
// source identity. This is a method/structure constraint, not a semantic
// judgment about the original criterion or an execution failure.
func shellNativeUnmanagedSelfExecution(file *ast.File) bool {
	aliases := map[string]bool{}
	for _, im := range file.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		if p == "os" {
			name := "os"
			if im.Name != nil {
				name = im.Name.Name
			}
			aliases[name] = true
		}
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Executable" {
				if base, ok := sel.X.(*ast.Ident); ok && base.Obj == nil && aliases[base.Name] {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// Entrypoint review observes existing input bytes under the declared scope;
// it does not rewrite them or infer assertion semantics.
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
