package shell

import (
	"fmt"
	"go/ast"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellGoIsolatedEntrypointObservationProtocolRef = "shell/go-test-json/workspace-behavior/v5"

const shellIsolatedInputContentContractRef = "shell/go-test-input/isolated-entrypoint/v1"
const shellIsolatedReviewScopeContractRef = "shell/go-test-input/isolated-package-review/v1"
const shellIsolatedReviewScopeRequirements = "The supplied files are the complete current CWI input package, including its helpers/fixtures and present root module/workspace definitions. Shell executes only this package's exact original and supplemental native tests, not the project-wide test suite or neighboring validation packages. Root GOWORK is disabled. Go-built children inherit owner instrumentation; only actual execution of non-test, non-harness workspace implementation yields behavior grounding, never test success alone. An original full-suite/build criterion requires its own explicit execution and assertions; this package-level run cannot satisfy it by implication or omit existing failures. Inputs that depend on unreviewed external harness code or fixtures cannot be judged consistent from this subject; identify the missing context. Package scope is not an OS filesystem/process sandbox. Every original Goal/DoD criterion remains mandatory; the scope grants neither criterion replacement nor acceptance credit."
const shellIsolatedInputContentRequirements = "Create exactly targetRef as a Go test in its own package directory, with the exact nativeCheckRef func(t *testing.T). Named same-package helpers, types, constants and data-only variables are allowed; no init, TestMain, methods, other Test functions, global initializer calls or closures, nested modules, cgo or imports of other validation packages. Keep all check helpers and fixtures inside this package directory; its complete contents and root module definitions must fit the owner review envelope (16 files, 64 KiB). Tests run from this directory, three levels below the workspace root. Exercise the actual workspace delivery in-process or via a freshly Go-built executable inheriting owner GOFLAGS and GOCOVERDIR; do not use prebuilt binaries, dependency-only or test-local substitutes, source markers, instrumentation overrides or input mutation. Assert the original criterion and every required case through the real testing.T; missing dependencies, skips and unobserved delivery are unverified, never passed. All original Goal/DoD criteria remain mandatory; do not weaken or edit historical inputs. The owner selects only this check package, not a full-project test suite: any original full-suite/build requirement still needs its own actual execution and assertions. Inputs depending on unreviewed external harness code or fixtures cannot be declared consistent. Creation, structural readiness and consistency review grant no execution or acceptance credit."

var shellIsolatedNativeIdentity = regexp.MustCompile(`^TestWorkOrchCWI_[A-Z0-9_]+_InputV4$`)

func shellValidationInputReviewRequired(adapter string) bool {
	return adapter == modulecommon.ValidationRecipeAdapterVersionedEntrypoint || adapter == modulecommon.ValidationRecipeAdapterIsolatedEntrypoint || adapter == modulecommon.ValidationRecipeAdapterConjunctiveEntrypoint || adapter == modulecommon.ValidationRecipeAdapterProjectConjunction
}

func shellEntrypointObservationProtocol(ref string) bool {
	return ref == shellGoEntrypointObservationProtocolRef || ref == shellGoIsolatedEntrypointObservationProtocolRef || ref == shellGoConjunctiveEntrypointObservationProtocolRef || ref == shellProjectConjunctionProtocolRef
}

func shellIsolatedNativeCheck(check runtimecontract.RuntimeValidationCheckAttestationBinding) bool {
	return check.ObservationProtocolRef == shellGoIsolatedEntrypointObservationProtocolRef && shellIsolatedNativeIdentity.MatchString(strings.TrimSuffix(check.NativeCheckRef, "_Behavior"))
}

func shellIsolatedNativePackage(native string) string {
	return "workorch_validation/v4/" + strings.ToLower(strings.TrimSuffix(native, "_Behavior"))
}

// v5 owns one exact Go package per immutable CWI mapping. Older protocols keep
// their original package selection and namespace-wide review semantics.
func shellIsolatedValidationPackage(checks []runtimecontract.RuntimeValidationCheckAttestationBinding) (string, error) {
	for _, check := range checks {
		if check.ObservationProtocolRef == shellProjectConjunctionProtocolRef {
			pkg := shellProjectNativePackage(check.NativeCheckRef)
			if pkg == "" || strings.HasSuffix(check.NativeCheckRef, "_Behavior") || len(checks) > maxShellBehaviorNativeChecks {
				return "", fmt.Errorf("project conjunction requires exact bounded native mappings")
			}
			hash := runtimecontract.StableRuntimeValidationExecutionStagesHash(shellProjectExecutionStageIdentities(pkg))
			var required []string
			for _, member := range checks {
				if member.ObservationProtocolRef != shellProjectConjunctionProtocolRef || shellProjectNativePackage(member.NativeCheckRef) != pkg || member.ExecutionStagesHash != hash || strings.HasSuffix(member.NativeCheckRef, "_Behavior") {
					return "", fmt.Errorf("project conjunction cannot mix packages, protocols or execution stages")
				}
				required = append(required, member.RequiredCheckRef)
			}
			if err := runtimecontract.ValidateRuntimeValidationCheckAttestationBindings(checks, required); err != nil {
				return "", err
			}
			return pkg, nil
		}
	}
	for _, check := range checks {
		if check.ObservationProtocolRef == shellGoConjunctiveEntrypointObservationProtocolRef {
			pkg := shellConjunctiveNativePackage(check.NativeCheckRef)
			if pkg == "" || strings.HasSuffix(check.NativeCheckRef, "_Behavior") || len(checks) > maxShellBehaviorNativeChecks {
				return "", fmt.Errorf("conjunctive validation requires exact bounded native input mappings")
			}
			var required []string
			for _, member := range checks {
				if member.ObservationProtocolRef != shellGoConjunctiveEntrypointObservationProtocolRef ||
					shellConjunctiveNativePackage(member.NativeCheckRef) != pkg || strings.HasSuffix(member.NativeCheckRef, "_Behavior") {
					return "", fmt.Errorf("conjunctive validation cannot mix requirement packages or protocols")
				}
				required = append(required, member.RequiredCheckRef)
			}
			if err := runtimecontract.ValidateRuntimeValidationCheckAttestationBindings(checks, required); err != nil {
				return "", err
			}
			return pkg, nil
		}
	}
	for _, check := range checks {
		if check.ObservationProtocolRef == shellGoIsolatedEntrypointObservationProtocolRef {
			if len(checks) != 1 || !shellIsolatedNativeCheck(check) || strings.HasSuffix(check.NativeCheckRef, "_Behavior") {
				return "", fmt.Errorf("isolated validation requires one exact native input mapping")
			}
			return shellIsolatedNativePackage(check.NativeCheckRef), nil
		}
	}
	return "", nil
}

func shellIsolatedInputDeclarations(file *ast.File, native string) bool {
	if !shellEntrypointSupplementDeclarations(file, native) {
		return false
	}
	for _, item := range file.Imports {
		value, err := strconv.Unquote(item.Path.Value)
		if err != nil || value == "C" || strings.HasPrefix(value, ".") || strings.Contains("/"+value+"/", "/workorch_validation/") {
			return false
		}
	}
	return true
}

// Package membership, not a raised review limit, bounds independent CWIs. All
// helpers/fixtures in this package are reviewed; nested Go packages/modules are
// unsupported rather than silently omitted from the input closure.
func shellIsolatedInputScopeCode(refs []string, pkg string) string {
	for _, ref := range refs {
		base := path.Base(ref)
		configuration := base == "go.mod" || base == "go.sum" || base == "go.work" || base == "go.work.sum"
		if configuration && path.Dir(ref) != "." && strings.HasPrefix(pkg+"/", path.Dir(ref)+"/") {
			return "validation_input_review_scope_unsupported"
		}
		if !strings.HasPrefix(ref, pkg+"/") {
			continue
		}
		if configuration || (strings.HasSuffix(ref, ".go") && path.Dir(ref) != pkg) {
			return "validation_input_review_scope_unsupported"
		}
	}
	return ""
}

// Go loads a package from disk, not from a caller's file list. Before publishing
// review bytes or executing that package, require its actual bounded membership
// to match the bound File manifest. This is a capability fact, not a freshness
// retry or a state transition. No neighboring CWI directory is traversed.
func shellIsolatedInputSourceSetCode(root *os.Root, refs []string, pkg string) string {
	expected, actual := map[string]bool{}, map[string]bool{}
	for _, ref := range refs {
		if strings.HasPrefix(ref, pkg+"/") || ref == "go.mod" || ref == "go.sum" || ref == "go.work" || ref == "go.work.sum" {
			expected[ref] = true
		}
	}
	if len(expected) > runtimecontract.RuntimeValidationInputReviewMaxFiles {
		return "validation_input_review_capacity_exceeded"
	}
	for _, ref := range []string{"go.mod", "go.sum", "go.work", "go.work.sum"} {
		info, err := root.Lstat(ref)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return "validation_input_review_source_unavailable"
		}
		actual[ref] = true
	}
	var directories []string
	for current := pkg; current != "."; current = path.Dir(current) {
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			continue // An absent package is eligible for create-only preparation.
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "validation_input_review_source_unavailable"
		}
		for _, name := range []string{"go.mod", "go.sum", "go.work", "go.work.sum"} {
			if _, err := root.Lstat(path.Join(current, name)); !os.IsNotExist(err) {
				return "validation_input_review_scope_unsupported"
			}
		}
		if current == pkg {
			directories = append(directories, current)
		}
	}
	remaining := maxNativeCheckSourceFiles
	for len(directories) != 0 {
		dir := directories[len(directories)-1]
		directories = directories[:len(directories)-1]
		file, err := root.Open(dir)
		if err != nil {
			return "validation_input_review_source_unavailable"
		}
		entries, readErr := file.ReadDir(remaining + 1)
		file.Close()
		if readErr != nil && readErr != io.EOF {
			return "validation_input_review_source_unavailable"
		}
		remaining -= len(entries)
		if remaining < 0 {
			return "validation_input_review_capacity_exceeded"
		}
		for _, entry := range entries {
			ref := path.Join(dir, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 {
				return "validation_input_review_source_unavailable"
			}
			if entry.IsDir() {
				directories = append(directories, ref)
				continue
			}
			if !entry.Type().IsRegular() {
				return "validation_input_review_source_unavailable"
			}
			if code := shellIsolatedInputScopeCode([]string{ref}, pkg); code != "" {
				return code
			}
			actual[ref] = true
			if len(actual) > runtimecontract.RuntimeValidationInputReviewMaxFiles {
				return "validation_input_review_capacity_exceeded"
			}
		}
	}
	if len(actual) != len(expected) {
		return "validation_input_review_source_set_changed"
	}
	for ref := range actual {
		if !expected[ref] {
			return "validation_input_review_source_set_changed"
		}
	}
	return ""
}

// Use exact implementation package identities from the sealed manifest, not
// ./..., which can load unrelated validation packages. Module-qualified names
// also remain stable when a Go-built child changes its working directory.
func shellIsolatedCoveragePackages(directory string, refs []string, inputPackage string) (string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	file, err := root.Open("go.mod")
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	file.Close()
	if err != nil || len(body) > 64*1024 {
		return "", fmt.Errorf("isolated validation module identity unavailable")
	}
	module := ""
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.SplitN(line, "//", 2)[0])
		if len(fields) == 2 && fields[0] == "module" {
			module = strings.Trim(fields[1], "\"")
			break
		}
	}
	valid := func(value string) bool {
		return value != "" && !strings.Contains(value, "...") && strings.IndexFunc(value, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("./_-~", r))
		}) < 0
	}
	if !valid(module) {
		return "", fmt.Errorf("isolated validation requires an exact module identity")
	}
	var packages []string
	for _, ref := range normalizeShellStrings(refs) {
		if !strings.HasSuffix(ref, ".go") || strings.HasSuffix(ref, "_test.go") || strings.HasPrefix(ref, "workorch_validation/") || managedValidationSandboxPathExcluded(ref) {
			continue
		}
		pkg := module
		if dir := path.Dir(ref); dir != "." {
			pkg += "/" + dir
		}
		if !valid(pkg) {
			return "", fmt.Errorf("isolated validation requires exact implementation package identities")
		}
		packages = append(packages, pkg)
	}
	packages = normalizeShellStrings(packages)
	if len(packages) == 0 {
		// A test-only workspace may execute, but produces no delivery grounding.
		packages = []string{module + "/" + inputPackage}
	}
	result := strings.Join(packages, ",")
	if len(packages) > maxNativeCheckSourceFiles || len(result) > 32*1024 {
		return "", fmt.Errorf("isolated validation implementation package scope exceeds observation bound")
	}
	return result, nil
}
