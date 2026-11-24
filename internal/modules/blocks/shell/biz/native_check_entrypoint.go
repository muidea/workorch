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

// Shell executes managed Go checks with GOFLAGS=-trimpath from the package
// working directory; runtime.Caller/CallersFrames resolve module-relative paths
// rather than workspace filesystem locations. This guards validation harness
// inputs only and is not a semantic judgment about user assertions or failures.
func shellNativeTrimpathCallerPath(file *ast.File) bool {
	aliases := map[string]bool{}
	dotImport := false
	for _, im := range file.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		if p != "runtime" {
			continue
		}
		if im.Name == nil {
			aliases["runtime"] = true
		} else if im.Name.Name == "." {
			dotImport = true
		} else if im.Name.Name != "_" {
			aliases[im.Name.Name] = true
		}
	}
	if len(aliases) == 0 && !dotImport {
		return false
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fn.Sel.Name == "Caller" || fn.Sel.Name == "CallersFrames" {
				if base, ok := fn.X.(*ast.Ident); ok && base.Obj == nil && aliases[base.Name] {
					found = true
				}
			}
		case *ast.Ident:
			if dotImport && fn.Obj == nil && (fn.Name == "Caller" || fn.Name == "CallersFrames") {
				found = true
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
