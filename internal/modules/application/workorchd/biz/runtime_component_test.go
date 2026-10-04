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
	"time"

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

func TestRuntimeComponentRetainsDependenciesUntilAuthoringDrains(t *testing.T) {
	component := newRuntimeComponent(&runtimecore.Session{})
	ctx, release, err := component.authoringTasks.accept(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	settlement, cancelSettlement := authoringSettlementContext(ctx)
	defer cancelSettlement()
	deadline, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := component.close(deadline); err == nil {
		t.Fatal("未排空创作调用却报告已关闭")
	}
	if !component.ready() || component.applicationPorts().Validate() != nil {
		t.Fatal("排空超时清除了在途调用依赖")
	}
	select {
	case <-settlement.Done():
	case <-time.After(time.Second):
		t.Fatal("结果结算未遵守唯一停机窗口")
	}
	release()
	if err := component.close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if component.ready() {
		t.Fatal("排空后仍保留 session")
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
