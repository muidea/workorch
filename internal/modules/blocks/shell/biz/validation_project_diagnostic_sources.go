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
const shellProjectMultiFailureRequirements = "Review every package-qualified native source against immutable Goal/DoD using the complete project input corpus: all tests, validation helpers, fixtures/docs and root module/workspace definitions (64 files/256 KiB, 64 sources, report 64 KiB). Non-test non-harness Go is mutable delivery, separately observed by execution/source integrity. Unreviewed external harness dependencies, omitted/unknown/ambiguous/conflicting/over-capacity sources, missing assertions or placeholder failures block consistency; each source needs its own finding. These additional all-of constraints preserve every original check/criterion; they do not replace CWI checks. Root build/test/race and independent reviewed CWI behavior remain mandatory. On exact unchanged root test/race failure, at most five failed top-level sources reproduce concurrently in complete separate bounded workspace copies/environments/instrumentation under the original operation deadline. Join/cleanup every started process. Require every original failed case and complete failure-identity union plus independent real implementation behavior per source; no borrowed coverage, guessed targets, extra operation budget or diagnostic credit. Missing/skip/build-failed/unknown/drift/cleanup/timeout remains closed. In-process delivery and fresh Go-built delivery executables inheriting owner GOFLAGS/GOCOVERDIR are supported; prebuilt substitutes, instrumentation overrides or input mutation are forbidden. Edit targets do not restrict observation methods. Missing delivery or real behavior failure alone is not input inconsistency: judge original obligations, leave execution to validation. Review grants neither mutation nor acceptance." + shellNativeFailureObservationRequirements + shellProjectProofTopologyRequirements + shellNativeSourceExecutionRequirements

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
			if fn.Recv != nil {
				continue
			}
			if name == "TestMain" || strings.HasPrefix(name, "Fuzz") || strings.HasPrefix(name, "Example") {
				return nil, "validation_project_diagnostic_native_unsupported"
			}
			if !shellGoTestName(name) {
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
