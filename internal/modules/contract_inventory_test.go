package modules

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

var componentContractVerifiedPattern = regexp.MustCompile(`(?m)^Last Verified: \d{4}-\d{2}-\d{2}$`)
var componentContractHeadingPattern = regexp.MustCompile(`(?m)^## [^\r\n]+$`)
var componentContractTestNamePattern = regexp.MustCompile(`\bTest[A-Za-z0-9_]+\b`)
var componentContractTestDefinitionPattern = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\s*\(`)
var componentContractLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

var componentContractHeadings = []string{
	"## 功能范围与边界",
	"## 组件交互及合同",
	"## 不变量与非职责",
	"## 能力验收",
}

// The executable registration baseline must be available in a clean checkout.
// Local design documents under docs/ are deliberately not versioned inputs.
var componentContractInventory = []string{
	"internal/initiators/workorchruntime",
	"internal/modules/application/workorchd",
	"internal/modules/blocks/approval",
	"internal/modules/blocks/decision",
	"internal/modules/blocks/eventstream",
	"internal/modules/blocks/file",
	"internal/modules/blocks/git",
	"internal/modules/blocks/llm",
	"internal/modules/blocks/mcp",
	"internal/modules/blocks/mcpregistry",
	"internal/modules/blocks/memory",
	"internal/modules/blocks/result",
	"internal/modules/blocks/run",
	"internal/modules/blocks/shell",
	"internal/modules/blocks/skill",
	"internal/modules/blocks/soul",
	"internal/modules/blocks/ssh",
	"internal/modules/blocks/web",
	"internal/modules/kernel/governance",
	"internal/modules/kernel/orchestrator",
	"internal/modules/kernel/system",
	"internal/modules/kernel/workspace",
}

func TestComponentContractMatrixCoversRegisteredComponents(t *testing.T) {
	root := findRepoRoot(t)
	testSymbols := repositoryTestSymbols(t, root)

	registered := registeredComponentDirs(t, root)
	if !slices.Equal(registered, componentContractInventory) {
		t.Fatalf("registered components = %q, want executable baseline %q", registered, componentContractInventory)
	}
	if len(registered) != 22 {
		t.Fatalf("registered component count = %d, want current 22-unit baseline", len(registered))
	}
	var initiators, modules, blocks int
	for _, rel := range registered {
		switch {
		case strings.HasPrefix(rel, "internal/initiators/"):
			initiators++
		case strings.HasPrefix(rel, "internal/modules/blocks/"):
			blocks++
		default:
			modules++
		}
		readmeRel := rel + "/readme.md"
		readmePath := filepath.Join(root, filepath.FromSlash(readmeRel))
		readmeBytes, err := os.ReadFile(readmePath)
		if err != nil {
			t.Fatalf("registered component %s has no readable contract: %v", rel, err)
		}
		assertComponentContractShape(t, root, readmeRel, string(readmeBytes), testSymbols)
	}
	if initiators != 1 || modules != 5 || blocks != 16 {
		t.Fatalf("registered unit classification = %d initiator, %d modules, %d blocks; want 1/5/16", initiators, modules, blocks)
	}
}

func assertComponentContractShape(t *testing.T, root, readmeRel, contract string, testSymbols map[string]struct{}) {
	t.Helper()
	for _, metadata := range []string{"Status: active", "Type:"} {
		if !strings.Contains(contract, metadata) {
			t.Errorf("component contract %s is missing metadata %q", readmeRel, metadata)
		}
	}
	if !componentContractVerifiedPattern.MatchString(contract) {
		t.Errorf("component contract %s has no canonical Last Verified date", readmeRel)
	}
	for _, heading := range componentContractHeadings {
		if count := strings.Count(contract, heading); count != 1 {
			t.Errorf("component contract %s heading %q count = %d, want 1", readmeRel, heading, count)
		}
	}
	headings := componentContractHeadingPattern.FindAllString(contract, -1)
	if len(headings) != len(componentContractHeadings) {
		t.Errorf("component contract %s top-level headings = %q, want only canonical four", readmeRel, headings)
	} else {
		for index, heading := range componentContractHeadings {
			if headings[index] != heading {
				t.Errorf("component contract %s heading[%d] = %q, want %q", readmeRel, index, headings[index], heading)
			}
		}
	}
	for _, noise := range []string{
		"## 收口结论",
		"## 当前验证结论",
		"## 扩展准入条件",
		"当前没有已知的本地可执行合同或测试缺口",
		"当前没有与本合同对应的已知可执行缺口",
		"- [x]",
	} {
		if strings.Contains(contract, noise) {
			t.Errorf("component contract %s retains non-normative progress noise %q", readmeRel, noise)
		}
	}
	testNames := componentContractTestNamePattern.FindAllString(contract, -1)
	if !strings.Contains(contract, "_test.go") && !strings.Contains(contract, "make verify-") && len(testNames) == 0 {
		t.Errorf("component contract %s does not map acceptance to executable evidence", readmeRel)
	}
	for _, testName := range testNames {
		if _, found := testSymbols[testName]; !found {
			t.Errorf("component contract %s cites missing test %s", readmeRel, testName)
		}
	}
	for _, match := range componentContractLinkPattern.FindAllStringSubmatch(contract, -1) {
		target := strings.TrimSpace(strings.SplitN(match[1], "#", 2)[0])
		if target == "" || strings.Contains(target, "://") {
			continue
		}
		resolved := filepath.Clean(filepath.Join(root, filepath.Dir(readmeRel), filepath.FromSlash(target)))
		if componentContractLocalDocsUnavailable(root, resolved) {
			continue // Local docs are optional; source/test links remain required.
		}
		if _, err := os.Stat(resolved); err != nil {
			t.Errorf("component contract %s links missing local target %q: %v", readmeRel, target, err)
		}
	}
}

func componentContractLocalDocsUnavailable(root, resolved string) bool {
	docs := filepath.Join(root, "docs")
	relative, err := filepath.Rel(docs, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	_, err = os.Stat(docs)
	return os.IsNotExist(err)
}

func TestComponentContractLocalDocsAreOptionalOnlyOutsideSource(t *testing.T) {
	root := t.TempDir()
	doc := filepath.Join(root, "docs", "design.md")
	if !componentContractLocalDocsUnavailable(root, doc) {
		t.Fatal("clean checkout without local docs must retain executable checks")
	}
	for _, path := range []string{filepath.Join(root, "internal", "owner.go"), filepath.Join(root, "docs-other", "design.md")} {
		if componentContractLocalDocsUnavailable(root, path) {
			t.Fatal("missing source or adjacent path was treated as optional documentation")
		}
	}
	if err := os.Mkdir(filepath.Join(root, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	if componentContractLocalDocsUnavailable(root, doc) {
		t.Fatal("present local documentation must still have its links checked")
	}
}

func repositoryTestSymbols(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	ret := map[string]struct{}{}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range componentContractTestDefinitionPattern.FindAllSubmatch(content, -1) {
			ret[string(match[1])] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inventory repository test symbols: %v", err)
	}
	return ret
}

func registeredComponentDirs(t *testing.T, root string) []string {
	t.Helper()

	var dirs []string
	for _, base := range []string{"internal/initiators", "internal/modules"} {
		err := filepath.WalkDir(filepath.Join(root, base), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Name() != "module.go" {
				return nil
			}
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			dirs = append(dirs, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}

	sort.Strings(dirs)
	return dirs
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("go.mod not found from %s", dir)
		}
		dir = next
	}
}
