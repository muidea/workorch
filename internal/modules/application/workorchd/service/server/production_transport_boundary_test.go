package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductionTransportDoesNotImportRuntimeSessionEventHubOrRootAppservice(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range entries {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		unit, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range unit.Imports {
			importPath := strings.Trim(spec.Path.Value, "\"")
			if strings.HasSuffix(importPath, "/biz/appservice/runtimecore") ||
				strings.HasSuffix(importPath, "/magicCommon/event") ||
				strings.HasSuffix(importPath, "/modules/application/workorchd/biz/appservice") ||
				((strings.Contains(importPath, "/internal/modules/blocks/") || strings.Contains(importPath, "/internal/modules/kernel/")) &&
					(strings.Contains(importPath, "/biz") || strings.Contains(importPath, "/service"))) {
				t.Errorf("production transport %s imports forbidden runtime boundary %q", filepath.Base(path), importPath)
			}
		}
	}
}

func TestProductionTransportDoesNotRetainRuntimeSessionCompatibilityState(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range entries {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		unit, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(unit, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if ident.Name == "sharedSession" ||
				ident.Name == "runtimeSessionErr" ||
				ident.Name == "hasSharedSession" ||
				ident.Name == "withSharedSession" ||
				ident.Name == "workspaceProjectionObserver" ||
				ident.Name == "workspaceProjectionLifecycle" ||
				ident.Name == "manageWorkspaceProjectionLifecycle" ||
				ident.Name == "newWorkspaceProjectionManager" ||
				ident.Name == "productionWiring" ||
				strings.HasPrefix(ident.Name, "compatRead") {
				t.Errorf("production transport %s retains forbidden compatibility identifier %q", filepath.Base(path), ident.Name)
			}
			return true
		})
	}
}
