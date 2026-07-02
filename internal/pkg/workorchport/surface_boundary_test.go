package workorchport

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSurfacePackagesKeepLayeredImportBoundary(t *testing.T) {
	repoRoot := surfaceBoundaryRepoRoot(t)
	tests := []struct {
		name       string
		paths      []string
		forbidden  []string
		requireAny []string
	}{
		{
			name:  "workorchport is DTO and backend contract only",
			paths: []string{"internal/pkg/workorchport"},
			forbidden: []string{
				"github.com/muidea/magicCommon/",
				"github.com/muidea/workorch/internal/modules/",
				"github.com/muidea/workorch/internal/pkg/repository",
				"github.com/muidea/workorch/internal/pkg/workorchclient",
				"github.com/muidea/workorch/internal/services/workorch",
			},
		},
		{
			name:  "workorchclient is REST client only",
			paths: []string{"internal/pkg/workorchclient"},
			forbidden: []string{
				"github.com/muidea/magicCommon/",
				"github.com/muidea/workorch/internal/modules/",
				"github.com/muidea/workorch/internal/pkg/repository",
				"github.com/muidea/workorch/internal/pkg/workorchport",
				"github.com/muidea/workorch/internal/services/workorch",
			},
		},
		{
			name: "interactive surfaces use port contract",
			paths: []string{
				"internal/services/workorch/biz/agent",
				"internal/services/workorch/biz/session",
				"internal/services/workorch/service/cli",
				"internal/services/workorch/service/tui",
			},
			forbidden: []string{
				"github.com/muidea/magicCommon/",
				"github.com/muidea/workorch/internal/modules/",
				"github.com/muidea/workorch/internal/pkg/repository",
				"github.com/muidea/workorch/internal/pkg/workorchclient",
			},
			requireAny: []string{"github.com/muidea/workorch/internal/pkg/workorchport"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			imports := surfaceBoundaryImports(t, repoRoot, tc.paths)
			for file, paths := range imports {
				for _, importPath := range paths {
					for _, forbidden := range tc.forbidden {
						if strings.HasPrefix(importPath, forbidden) {
							t.Fatalf("%s imports forbidden dependency %q", file, importPath)
						}
					}
				}
			}
			if len(tc.requireAny) == 0 {
				return
			}
			for _, paths := range imports {
				for _, importPath := range paths {
					for _, required := range tc.requireAny {
						if importPath == required {
							return
						}
					}
				}
			}
			t.Fatalf("none of %v imports required contract package %v", tc.paths, tc.requireAny)
		})
	}
}

func surfaceBoundaryImports(t *testing.T, repoRoot string, relPaths []string) map[string][]string {
	t.Helper()
	ret := map[string][]string{}
	for _, relPath := range relPaths {
		dir := filepath.Join(repoRoot, relPath)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			imports := make([]string, 0, len(file.Imports))
			for _, item := range file.Imports {
				imports = append(imports, strings.Trim(item.Path.Value, `"`))
			}
			ret[filepath.ToSlash(relPath+"/"+entry.Name())] = imports
		}
	}
	return ret
}

func surfaceBoundaryRepoRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../.."))
}
