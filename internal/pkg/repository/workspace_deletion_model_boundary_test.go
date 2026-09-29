package repository_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func isIsolatedWorktreeDirectory(root, path string) bool {
	return filepath.Clean(path) == filepath.Join(root, ".claude", "worktrees")
}

func TestOwnerBoundaryWorktreeExclusionDoesNotExcludeProduction(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		path string
		skip bool
	}{
		{filepath.Join(".claude", "worktrees"), true},
		{".claude", false},
		{"internal", false},
		{filepath.Join("internal", ".claude", "worktrees"), false},
		{filepath.Join("internal", "modules", "blocks", "workspace"), false},
	} {
		if got := isIsolatedWorktreeDirectory(root, filepath.Join(root, tc.path)); got != tc.skip {
			t.Errorf("scan exclusion %q = %v, want %v", tc.path, got, tc.skip)
		}
	}
}

// WorkspaceDeletion is an owner-private persistence entity. EventHub exposes
// only immutable concrete acknowledgement DTOs; sibling production packages
// must not construct or reduce the durable tombstone directly.
func TestWorkspaceDeletionPersistenceModelsStayOwnerPrivate(t *testing.T) {
	repositoryRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"internal/modules/blocks/memory/biz/workspace_deletion_internal_event.go":   true,
		"internal/modules/blocks/result/biz/workspace_deletion_internal_event.go":   true,
		"internal/modules/blocks/decision/biz/workspace_deletion_internal_event.go": true,
		"internal/modules/blocks/approval/biz/workspace_deletion_internal_event.go": true,
		"internal/pkg/repository/memory/memory.go":                                  true,
		"internal/pkg/workorchrepo/workspace_deletion_repositories.go":              true,
	}
	modelPackages := map[string]bool{
		"github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models":   true,
		"github.com/muidea/workorch/internal/modules/blocks/result/pkg/models":   true,
		"github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models": true,
		"github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models": true,
	}
	err = filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			// 隔离工作树不是当前仓库源码；仅排除其明确路径，不豁免主树 owner 检查。
			if isIsolatedWorktreeDirectory(repositoryRoot, path) {
				return filepath.SkipDir
			}
			// Runtime data and generated dependency trees are not repository
			// source. In particular, an in-progress workspace may contain
			// intentionally incomplete Go files and must never weaken or break
			// this static owner-boundary scan.
			if entry.Name() == ".git" || entry.Name() == "node_modules" ||
				entry.Name() == ".workorch-data" || entry.Name() == ".workorch-workspaces" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if allowed[relative] {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		aliases := map[string]bool{}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil || !modelPackages[importPath] {
				continue
			}
			alias := filepath.Base(importPath)
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			aliases[alias] = true
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "WorkspaceDeletion" {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if ok && aliases[qualifier.Name] {
				t.Errorf("%s constructs owner-private %s.WorkspaceDeletion", relative, qualifier.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
