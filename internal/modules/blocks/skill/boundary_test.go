package skill

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestRuntimeCoreAndLLMDoNotReadSkillRegistryDirectly(t *testing.T) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve boundary test path")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	targets := []string{
		filepath.Join(repositoryRoot, "internal", "modules", "kernel", "orchestrator"),
		filepath.Join(repositoryRoot, "internal", "modules", "blocks", "llm"),
	}
	for _, target := range targets {
		err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, imported := range file.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if strings.Contains(importPath, "/internal/modules/blocks/skill/") {
					t.Errorf("%s imports Skill owner package %q", path, importPath)
				}
			}
			ast.Inspect(file, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.SelectorExpr:
					if value.Sel.Name == "SkillRepository" {
						t.Errorf("%s directly references SkillRepository", path)
					}
				case *ast.BasicLit:
					if value.Kind != token.STRING {
						break
					}
					literal, err := strconv.Unquote(value.Value)
					if err == nil && strings.HasPrefix(literal, "/skill/command/") {
						t.Errorf("%s embeds Skill owner EventHub topic %q", path, literal)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", target, err)
		}
	}
}
