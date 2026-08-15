package biz

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
)

func TestRuntimeComponentIsTheOnlySessionOwner(t *testing.T) {
	component := newRuntimeComponent(&runtimecore.Session{})
	if !component.ready() {
		t.Fatal("runtime component is not ready")
	}
	if err := component.applicationPorts().Validate(); err != nil {
		t.Fatalf("runtime component ports are incomplete: %v", err)
	}
	if err := component.close(context.Background()); err != nil {
		t.Fatalf("close runtime component: %v", err)
	}
	if component.ready() {
		t.Fatal("runtime component retained session after close")
	}
}

func TestBizProductionHasOneRuntimeSessionStorageBoundary(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)
	allowed := filepath.Join(root, "runtime_component.go")
	appserviceRoot := filepath.Join(root, "appservice")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == appserviceRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || path == allowed {
			return nil
		}
		unit, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		if runtimeSessionField(unit) {
			t.Errorf("Biz production file %s stores or exposes runtimecore.Session; use runtimeComponent", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Biz runtime ownership: %v", err)
	}
}

func runtimeSessionField(unit *ast.File) bool {
	for _, decl := range unit.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok.String() != "type" {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					if name.Name != "session" {
						continue
					}
					star, ok := field.Type.(*ast.StarExpr)
					if !ok {
						continue
					}
					selector, ok := star.X.(*ast.SelectorExpr)
					if ok && selector.Sel.Name == "Session" {
						if _, ok := selector.X.(*ast.Ident); ok {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
