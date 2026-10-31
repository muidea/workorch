package shell

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const maxShellBehaviorNativeChecks = 32

// Observation identities distinguish direct command output, delivery behavior,
// supplements and entrypoint isolation. They describe different capabilities,
// not historical versions. Behavior checks require actual non-test execution;
// none authorizes edits to the assertions or replaces semantic review.
func shellNativeObservationProtocolSupported(ref string) bool {
	return ref == shellGoTestObservationProtocolRef || ref == shellGoBehaviorObservationProtocolRef || ref == shellGoSupplementObservationProtocolRef || shellEntrypointObservationProtocol(ref)
}

func shellRequiresWorkspaceBehavior(request RuntimeCommandRequest) bool {
	for _, attestation := range request.ValidationCheckAttestations {
		if attestation.ObservationProtocolRef == shellGoBehaviorObservationProtocolRef || attestation.ObservationProtocolRef == shellGoSupplementObservationProtocolRef || shellEntrypointObservationProtocol(attestation.ObservationProtocolRef) {
			return true
		}
	}
	return false
}

func validateShellBehaviorRecipeArgs(args []string) error {
	if len(args) == 0 || args[0] != "test" {
		return fmt.Errorf("workspace behavior observation requires go test")
	}
	for _, arg := range args[1:] {
		flag := strings.SplitN(strings.TrimLeft(arg, "-"), "=", 2)[0]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		flag = strings.TrimPrefix(flag, "test.")
		switch flag {
		case "cover", "coverpkg", "coverprofile", "covermode", "exec", "overlay", "toolexec", "args", "c", "list", "bench", "fuzz", "fuzztime", "fuzzminimizetime":
			return fmt.Errorf("workspace behavior observation owns test instrumentation; recipe flag %s is unsupported", flag)
		case "run":
			if arg != "-run" {
				return fmt.Errorf("workspace behavior observation owns exact native-check selection")
			}
		case "count":
			if arg != "-count=1" {
				return fmt.Errorf("workspace behavior observation requires a fresh single execution")
			}
		case "json":
			if arg != "-json" {
				return fmt.Errorf("workspace behavior observation requires structured test output")
			}
		}
	}
	return nil
}

func prepareShellBehaviorObservation(request RuntimeCommandRequest, args []string, executionDirectories ...string) ([]string, string, error) {
	if !shellRequiresWorkspaceBehavior(request) {
		return args, "", nil
	}
	if !shellInputHasManagedValidation(request) || len(request.ValidationCheckAttestations) > maxShellBehaviorNativeChecks || len(args) == 0 || args[0] != "test" {
		return nil, "", fmt.Errorf("workspace behavior observation requires a bounded sealed managed native-check set")
	}
	if err := validateShellBehaviorRecipeArgs(args); err != nil {
		return nil, "", err
	}
	exactRun, jsonOutput, uncached := false, false, false
	var nativeChecks []string
	for _, attestation := range request.ValidationCheckAttestations {
		if attestation.ObservationProtocolRef != request.ValidationCheckAttestations[0].ObservationProtocolRef {
			return nil, "", fmt.Errorf("workspace behavior observation cannot mix native protocols")
		}
		for _, ref := range attestation.NativeCheckRefs() {
			nativeChecks = append(nativeChecks, regexp.QuoteMeta(ref))
		}
	}
	if len(nativeChecks) > maxShellBehaviorNativeChecks {
		return nil, "", fmt.Errorf("native input bound exceeded")
	}
	expectedRun := "^(" + strings.Join(normalizeShellStrings(nativeChecks), "|") + ")$"
	for index, arg := range args {
		if arg == "-run" {
			if exactRun || index+1 >= len(args) || args[index+1] != expectedRun {
				return nil, "", fmt.Errorf("workspace behavior observation requires one exact native-check selector")
			}
			exactRun = true
		}
		jsonOutput = jsonOutput || arg == "-json"
		uncached = uncached || arg == "-count=1"
	}
	if !exactRun || !jsonOutput || !uncached {
		return nil, "", fmt.Errorf("workspace behavior observation requires exact owner native-check instrumentation")
	}
	coveragePackages := "./..."
	pkg, err := shellIsolatedValidationPackage(request.ValidationCheckAttestations)
	if err != nil {
		return nil, "", err
	}
	if pkg != "" {
		prefix := "test\x00-json\x00-count=1\x00-run\x00" + expectedRun + "\x00"
		if joined := strings.Join(args, "\x00"); joined != prefix+"./"+pkg && joined != prefix+"-v\x00./"+pkg {
			return nil, "", fmt.Errorf("isolated validation must execute only its exact reviewed package")
		}
		directory := request.WorkspaceDeliverableDirectory
		if len(executionDirectories) != 0 {
			directory = executionDirectories[0]
		}
		if inspection := inspectShellNativeValidationChecks(directory, request.ValidationScopeTargetRefs, request.ValidationCheckAttestations); inspection.Code != "" {
			return nil, "", fmt.Errorf("isolated validation input precondition failed: %s", inspection.Code)
		}
		coveragePackages, err = shellIsolatedCoveragePackages(directory, request.ValidationScopeTargetRefs, pkg)
		if err != nil {
			return nil, "", err
		}
	}
	file, err := os.CreateTemp(request.ValidationScratchDirectory, "native-behavior-*.cover")
	if err != nil {
		return nil, "", err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		return nil, "", err
	}
	instrumented := []string{args[0], "-coverpkg=" + coveragePackages, "-covermode=atomic", "-coverprofile=" + name}
	return append(instrumented, args[1:]...), name, nil
}

func shellBehaviorEnvironment(env []string) []string {
	ret := make([]string, 0, len(env)+2)
	for _, value := range env {
		if strings.HasPrefix(value, "GOFLAGS=") || strings.HasPrefix(value, "GOWORK=") {
			continue
		}
		ret = append(ret, value)
	}
	return append(ret, "GOFLAGS=", "GOWORK=off")
}

// The coverage file is tool-produced inside the owner session, not stdout or
// a caller-selected path. Only exact files from the sealed File manifest in
// this module can count. Tests, the native harness and external dependencies
// cannot supply workspace implementation behavior.
func shellWorkspaceBehaviorTargets(directory, coveragePath string, refs []string) ([]string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	moduleFile, err := root.Open("go.mod")
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(moduleFile, 64*1024+1))
	moduleFile.Close()
	if err != nil || len(body) > 64*1024 {
		return nil, fmt.Errorf("workspace module identity is unavailable or exceeds observation bound")
	}
	modulePath := ""
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.SplitN(line, "//", 2)[0])
		if len(fields) == 2 && fields[0] == "module" {
			modulePath = strings.Trim(fields[1], "\"")
			break
		}
	}
	if modulePath == "" {
		return nil, fmt.Errorf("workspace module identity is unavailable")
	}
	allowed := make(map[string]string)
	for _, ref := range normalizeShellStrings(refs) {
		if strings.HasSuffix(ref, ".go") && !strings.HasSuffix(ref, "_test.go") &&
			!strings.HasPrefix(ref, "workorch_validation/") && !managedValidationSandboxPathExcluded(ref) {
			info, statErr := root.Stat(filepath.FromSlash(ref))
			if statErr == nil && info.Mode().IsRegular() {
				allowed[modulePath+"/"+ref] = ref
			}
		}
	}
	coverage, err := os.Open(coveragePath)
	if err != nil {
		return nil, err
	}
	defer coverage.Close()
	reader := &io.LimitedReader{R: coverage, N: maxNativeCheckSourceBytes + 1}
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() || (scanner.Text() != "mode: count" && scanner.Text() != "mode: atomic") {
		return nil, fmt.Errorf("workspace behavior coverage is missing its tool header")
	}
	var observed []string
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > maxNativeCheckWalkNodes {
			return nil, fmt.Errorf("workspace behavior coverage exceeds observation bound")
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return nil, fmt.Errorf("workspace behavior coverage record is invalid")
		}
		count, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("workspace behavior coverage count is invalid")
		}
		colon := strings.LastIndexByte(fields[0], ':')
		if colon < 0 {
			return nil, fmt.Errorf("workspace behavior coverage target is invalid")
		}
		if ref, ok := allowed[fields[0][:colon]]; ok && count > 0 {
			observed = append(observed, ref)
		}
	}
	if scanner.Err() != nil || reader.N <= 0 {
		return nil, fmt.Errorf("workspace behavior coverage is incomplete")
	}
	return normalizeShellStrings(observed), nil
}

func attachShellWorkspaceBehavior(output *RuntimeCommandResult, request RuntimeCommandRequest, executionDirectory, coveragePath string) error {
	if !shellRequiresWorkspaceBehavior(request) || output.ValidationObservation == nil {
		return nil
	}
	observation := output.ValidationObservation
	observation.ObservationProtocolRef = request.ValidationCheckAttestations[0].ObservationProtocolRef
	targets, observationErr := shellWorkspaceBehaviorTargets(executionDirectory, coveragePath, request.ValidationScopeTargetRefs)
	if shellRequiresEntrypointBehavior(request) {
		targets = canonicalShellBehaviorTargets(append(targets, output.workspaceProcessBehaviorTargetRefs...))
		if output.workspaceProcessBehaviorError != nil {
			observationErr = output.workspaceProcessBehaviorError
		}
		if len(output.workspaceProcessBehaviorTargetRefs) != 0 && observationErr == nil {
			output.EvidenceTags = appendUniqueShellEvidenceTag(output.EvidenceTags, "workspace_process_behavior_observed")
		}
	}
	if observationErr == nil {
		output.workspaceBehaviorTargetRefs = targets
	}
	// A process/build/test failure retains its original diagnostic. The
	// observation still supplies bounded implementation candidates, not the
	// assertion source, to the existing independent File confirmation lane.
	if output.ExitCode != 0 {
		return nil
	}
	if observationErr == nil && len(targets) != 0 && !output.OutputTruncated {
		output.EvidenceTags = appendUniqueShellEvidenceTag(output.EvidenceTags, "workspace_behavior_observed")
		return nil
	}
	code := "validation_workspace_behavior_unobserved"
	if observationErr != nil || output.OutputTruncated {
		code = "validation_workspace_behavior_unverifiable"
	}
	output.EvidenceQuality = string(common.EvidenceQualityInconclusive)
	output.EvidenceReason = "native check did not provide complete observed execution of the workspace implementation; source-text assertions and dependency-only tests are not delivery behavior proof"
	if (observation.ObservationProtocolRef == shellGoSupplementObservationProtocolRef || shellEntrypointHasSupplement(request)) && code == "validation_workspace_behavior_unobserved" {
		code = "validation_workspace_behavior_supplement_unobserved"
	}
	if code == "validation_workspace_behavior_unobserved" {
		for _, attestation := range request.ValidationCheckAttestations {
			output.EvidenceReason += fmt.Sprintf("; preserve existing checks; add %s with %s exercising the actual delivery entry for the same immutable requirement", shellSupplementTargetRef(attestation.NativeCheckRef), shellSupplementCheckRef(attestation.NativeCheckRef))
		}
	}
	output.FailureReason, output.FailureCode = code, code
	output.Stage = code
	for _, attestation := range request.ValidationCheckAttestations {
		observation.IncompleteChecks = append(observation.IncompleteChecks, attestation.NativeCheckRefs()...)
	}
	*observation = observation.Normalized()
	return fmt.Errorf("%s", output.EvidenceReason)
}
