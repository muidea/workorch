package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"slices"
	"strconv"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellProjectMultiFailureReviewScopeRef = "shell/go-test-input/project-multi-failure-review/v1"
const shellProjectMultiFailureRequirements = "Review the complete bounded project validation-input corpus (64 files/256 KiB, at most 64 native sources and a 64 KiB report): all project test files, validation namespace helpers, ordinary non-Go fixtures/documentation and root module/workspace definitions. Non-test non-harness Go files are mutable delivery implementation, NOT immutable review inputs; actual execution and source-integrity checks independently observe them. Any source depending on unreviewed external harness code or fixtures remains inconclusive, never inferred consistent. Independently review every owner-declared package-qualified native test against immutable Goal/DoD. These are compatible additional all-of project constraints, not guessed mappings or replacements for original CWI checks. Every source requires its own consistent finding; unknown, omitted, conflicting, ambiguous or over-capacity sources block. Preserve original tests. Root build/test/race and independently reviewed CWI behavior remain mandatory. Failed root test/race may reproduce at most five exact failed top-level native sources separately with owner instrumentation in the original operation deadline; each must reproduce every original failed case/assertion and independently observe real non-test/non-harness workspace implementation. Record each source and behavior scope with original raw streams; no aggregate coverage borrowing, guessed targets, credit or new operation budget. Missing, skip, build failure, unknown, drift, cleanup or timeout remain closed. Source review grants no input mutation or acceptance authority."

// Inventory comes from the exact bytes already returned for independent review,
// never a provider-supplied test name, marker or live directory guess.
func shellProjectDiagnosticSourceInventory(files []runtimecontract.RuntimeValidationInputReviewFile) ([]runtimecontract.RuntimeValidationDiagnosticSource, string) {
	module := ""
	for _, file := range files {
		if file.TargetRef == "go.mod" {
			for _, line := range strings.Split(file.Content, "\n") {
				fields := strings.Fields(strings.SplitN(line, "//", 2)[0])
				if len(fields) == 2 && fields[0] == "module" {
					if module != "" {
						return nil, "validation_project_diagnostic_module_ambiguous"
					}
					module = strings.Trim(fields[1], "\"")
				}
			}
		}
	}
	if module == "" {
		return nil, "validation_project_diagnostic_module_unavailable"
	}
	var result []runtimecontract.RuntimeValidationDiagnosticSource
	seen := map[string]bool{}
	for _, file := range files {
		if path.Base(file.TargetRef) == "go.mod" && file.TargetRef != "go.mod" {
			return nil, "validation_project_diagnostic_nested_module"
		}
		if !strings.HasSuffix(file.TargetRef, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file.TargetRef, file.Content, 0)
		if err != nil {
			return nil, "validation_project_diagnostic_input_invalid"
		}
		alias := "testing"
		for _, im := range parsed.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			if p == "testing" && im.Name != nil {
				alias = im.Name.Name
			}
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			name := fn.Name.Name
			if name == "TestMain" || strings.HasPrefix(name, "Fuzz") || strings.HasPrefix(name, "Example") {
				return nil, "validation_project_diagnostic_native_unsupported"
			}
			if !strings.HasPrefix(name, "Test") {
				continue
			}
			params := nativeCheckTestingParameters(fn.Type, alias)
			if fn.Recv != nil || fn.Type.Results != nil || fn.Type.Params == nil || fn.Type.Params.NumFields() != 1 || len(params) != 1 || params[0] == nil {
				return nil, "validation_project_diagnostic_native_unsupported"
			}
			pkg := module
			if dir := path.Dir(file.TargetRef); dir != "." {
				pkg += "/" + dir
			}
			native := pkg + "/" + name
			if seen[native] {
				return nil, "validation_project_diagnostic_native_ambiguous"
			}
			seen[native] = true
			sum := sha256.Sum256([]byte(native + "\x00" + file.TargetRef))
			result = append(result, runtimecontract.RuntimeValidationDiagnosticSource{SourceRef: "shell-project-source-" + hex.EncodeToString(sum[:]), NativeCaseRef: native, InputTargetRef: file.TargetRef})
			if len(result) > runtimecontract.RuntimeValidationDiagnosticSourceLimit {
				return nil, "validation_project_diagnostic_source_capacity_exceeded"
			}
		}
	}
	if len(result) == 0 {
		return nil, "validation_project_diagnostic_source_unavailable"
	}
	slices.SortFunc(result, func(a, b runtimecontract.RuntimeValidationDiagnosticSource) int {
		return strings.Compare(a.SourceRef, b.SourceRef)
	})
	return result, ""
}
