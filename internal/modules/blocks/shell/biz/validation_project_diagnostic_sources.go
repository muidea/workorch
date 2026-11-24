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
const shellProjectMultiFailureRequirements = "Review complete current package/helpers/fixtures, shared data/docs and root defs against original obligations (64 files/256 KiB, 64 sources, report 64 KiB); each diagnosticSources member needs a finding. Other packages are execution inputs only; never infer them. Missing bodies, external harness deps, unknown/ambiguous/conflicting/oversized sources, missing assertions and placeholders block consistency. Root build/test/race execute all tests, then independently observe exact CWI behavior; up to 5 failed sources reproduce concurrently in bounded copies under one deadline, joined/cleaned, preserving full failed cases/bodies/behavior (provenance grants no credit; missing/skip/build/unknown/drift/cleanup/timeout stay closed). Execution uses GOFLAGS=-trimpath, GOWORK=off in package cwd: locate go.mod via os.Getwd() (no runtime.Caller/CallersFrames), write temp/build files only to t.TempDir() (no workspace mutation), and exercise delivery in-process or run freshly go-built binaries inheriting owner env; owner GOCOVERDIR observes in-process calls or executed built binaries only, not go-build-only or nested go test. Prebuilt substitutes/overrides forbidden; edit targets do not restrict observations. Missing delivery or real behavior failure is not input inconsistency unless setup violates env/cwd assumptions. Review grants no mutation/acceptance." + shellNativeFailureObservationRequirements + shellProjectProofTopologyRequirements + shellNativeSourceExecutionRequirements

const shellProjectMultiFailureArtifactReviewScopeRef = "shell/go-test-input/project-multi-failure-artifact-review/v1"
const shellProjectMultiFailureArtifactReviewRequirements = "Review the complete current requirement package/helpers/fixtures, shared project data/docs and root module/workspace definitions against its original obligations (64 files/256 KiB, 64 reviewed sources, report 64 KiB). Each diagnosticSources member needs its own finding. Missing applicable bodies, external harness dependencies, unknown/ambiguous/conflicting/over-capacity sources, missing assertions, placeholders, private Runtime storage reads, or fabricated receipts block consistency. Execution uses GOFLAGS=-trimpath, GOWORK=off in package cwd: locate go.mod via os.Getwd() (no runtime.Caller/CallersFrames) and write temp/build files only to t.TempDir() (no workspace mutation). Root build/test/race execute ALL project tests, then reviewed-cwi-execution executes exact CWI checks (ProofClass executed, RequireBehavior false): verify actual workspace artifacts, static/lint checks, or owner obligations without requiring fake Go production coverage. At most five failed top-level sources independently reproduce concurrently in separate copies under one deadline with cleanup; missing/skip/build-failed/drift/timeout stay closed. Map each check to all obligations and required proof consumers; exact CWI execution, assessment and terminal receipt consumers remain mandatory and distinct. Judge obligations honestly; review grants no mutation or acceptance." + shellNativeFailureObservationRequirements + shellNativeSourceExecutionRequirements

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
