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
const shellProjectMultiFailureRequirements = "Review the complete current requirement package/helpers/fixtures, shared project data/docs and root module/workspace definitions against its original obligations (64 files/256 KiB, 64 reviewed sources, report 64 KiB). Each diagnosticSources member needs its own finding. Other packages are execution inputs, not this requirement's semantic obligations; do not rejudge historical inputs or infer their consistency. Missing applicable bodies, external harness dependencies, unknown/ambiguous/conflicting/over-capacity sources, missing assertions and placeholders block consistency. Mutable delivery is proved separately. Root build/test/race still freshly execute ALL project tests, including adjacent packages, then independently observe exact current CWI behavior. Execution owner observes actual source identities, not semantic PASS. At most five failed top-level sources independently reproduce concurrently in complete separate bounded workspace copies under the original deadline. Join and clean every process. Preserve every failed case, complete failure union, actual reproduction relation, affected input bodies and independent implementation behavior per source; provenance grants no repair or acceptance credit. Missing/skip/build-failed/unknown/drift/cleanup/timeout remain closed. In-process delivery and fresh Go-built executables inheriting owner instrumentation are supported; prebuilt substitutes, overrides and input mutation are forbidden. Edit targets do not restrict observations. Missing delivery or real behavior failure is not input inconsistency: judge the original obligations and leave execution to ordinary validation. Review grants no mutation or acceptance." + shellNativeFailureObservationRequirements + shellProjectProofTopologyRequirements + shellNativeSourceExecutionRequirements

const shellProjectMultiFailureArtifactReviewScopeRef = "shell/go-test-input/project-multi-failure-artifact-review/v1"
const shellProjectMultiFailureArtifactReviewRequirements = "Review the complete current requirement package/helpers/fixtures, shared project data/docs and root module/workspace definitions against its original obligations (64 files/256 KiB, 64 reviewed sources, report 64 KiB). Each diagnosticSources member needs its own finding. Missing applicable bodies, external harness dependencies, unknown/ambiguous/conflicting/over-capacity sources, missing assertions, placeholders, private Runtime storage reads, or fabricated receipts block consistency. Root build/test/race execute ALL project tests, then reviewed-cwi-execution executes exact CWI checks (ProofClass executed, RequireBehavior false): verify actual workspace artifacts, static/lint checks, or owner obligations without requiring fake Go production coverage. At most five failed top-level sources independently reproduce concurrently in separate copies under the original deadline with cleanup; missing/skip/build-failed/drift/timeout remain closed. Map each check to all applicable obligations and required proof consumers; exact CWI execution, assessment and terminal receipt consumers remain mandatory and distinct. Judge the original obligations honestly; review grants no mutation or acceptance." + shellNativeFailureObservationRequirements + shellNativeSourceExecutionRequirements

// Inventory comes from exact owner-observed bytes, for either the applicable
// review subject or the sealed execution snapshot. Never infer it from a
// provider-supplied test name, marker or live directory guess.
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
