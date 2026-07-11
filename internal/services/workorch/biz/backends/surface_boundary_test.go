package backend

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBackendSurfaceImportBoundary(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read backend dir: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		imports := parseGoImports(t, path)
		for _, imp := range imports {
			switch {
			case strings.Contains(imp, "/internal/pkg/repository"):
				t.Fatalf("%s imports repository package %q; backend surface must use port/client/EventHub contracts only", name, imp)
			case strings.Contains(imp, "/internal/modules/") && forbiddenBackendModuleImport(imp):
				t.Fatalf("%s imports forbidden module implementation package %q; backend surface may use event DTO packages only", name, imp)
			case isTechnicalRuntimeBlockImport(imp):
				t.Fatalf("%s imports runtime block package %q; backend surface must use workorchport runtime DTO helpers", name, imp)
			case strings.Contains(imp, "github.com/muidea/magicCommon/framework"):
				t.Fatalf("%s imports framework package %q; backend surface must not own framework wiring", name, imp)
			}
		}
	}
}

func TestRemoteAndEventHubBackendsStayOnSeparateAuthorityPaths(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)

	remoteImports := parseGoImports(t, filepath.Join(dir, "remote.go"))
	assertHasImport(t, "remote.go", remoteImports, "github.com/muidea/workorch/internal/pkg/workorchclient")
	assertNoImportContaining(t, "remote.go", remoteImports, "/internal/modules/")
	assertNoImportContaining(t, "remote.go", remoteImports, "/internal/pkg/workorchdeps")
	assertNoImportContaining(t, "remote.go", remoteImports, "/internal/pkg/repository")

	eventHubImports := parseGoImports(t, filepath.Join(dir, "eventhub.go"))
	assertNoImportContaining(t, "eventhub.go", eventHubImports, "/internal/pkg/workorchclient")
	assertNoImportContaining(t, "eventhub.go", eventHubImports, "/internal/pkg/repository")
	for _, imp := range eventHubImports {
		if strings.Contains(imp, "/internal/modules/") && forbiddenBackendModuleImport(imp) {
			t.Fatalf("eventhub.go imports forbidden module implementation package %q; EventHub backend may use event DTO packages only", imp)
		}
	}
}

func parseGoImports(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	imports := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		imports = append(imports, strings.Trim(spec.Path.Value, `"`))
	}
	return imports
}

func forbiddenBackendModuleImport(path string) bool {
	if strings.Contains(path, "/pkg/events") {
		return false
	}
	return strings.Contains(path, "/biz") ||
		strings.Contains(path, "/service") ||
		strings.Contains(path, "/repository") ||
		strings.Contains(path, "/pkg/models") ||
		strings.Contains(path, "/pkg/common")
}

func isTechnicalRuntimeBlockImport(path string) bool {
	for _, block := range []string{"file", "git", "llm", "mcp", "shell", "ssh", "web"} {
		root := "/internal/modules/blocks/" + block
		if strings.HasSuffix(path, root) || strings.Contains(path, root+"/") {
			return true
		}
	}
	return false
}

func assertHasImport(t *testing.T, file string, imports []string, want string) {
	t.Helper()
	for _, imp := range imports {
		if imp == want {
			return
		}
	}
	t.Fatalf("%s imports %v, want %q", file, imports, want)
}

func assertNoImportContaining(t *testing.T, file string, imports []string, forbidden string) {
	t.Helper()
	for _, imp := range imports {
		if strings.Contains(imp, forbidden) {
			t.Fatalf("%s imports %q, which contains forbidden boundary %q", file, imp, forbidden)
		}
	}
}
