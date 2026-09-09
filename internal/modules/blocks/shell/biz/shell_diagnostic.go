package shell

import (
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func attachShellCommandFailureDiagnostics(output *RuntimeCommandResult, execCommand string, err error) {
	if output == nil || err == nil || !shellCommandNotFound(err) {
		return
	}
	command := strings.TrimSpace(filepath.Base(execCommand))
	if command == "" {
		command = strings.TrimSpace(execCommand)
	}
	output.FailureReason = "command_not_found"
	output.UnavailableCommand = command
	output.CapabilityMissing = true
	output.RuntimeDiagnostics.Kind = "runtime_capability_unavailable"
	output.RuntimeDiagnostics.Operation = "shell/command"
	output.RuntimeDiagnostics.FailureReason = "command_not_found"
	output.RuntimeDiagnostics.UnavailableCommand = command
}

func attachShellValidationFailureDiagnostic(output *RuntimeCommandResult, request RuntimeCommandRequest, execCommand string, exitCode int, timedOut bool, policyViolation bool, err error) {
	if output == nil {
		return
	}
	class, targetType, ownerCode := shellValidationDiagnosticClassAndTarget(*output, request.ValidationEvidence, execCommand, timedOut, policyViolation, err)
	if class == "" {
		return
	}
	var targetRefs []string
	targetCandidatesValid := true
	if shellValidationDiagnosticMayIdentifyImplementationEffect(class) {
		targetRefs, targetCandidatesValid = shellValidationDiagnosticTargetCandidates(*output, request, execCommand, class)
	}
	if class == runtimemodel.ValidationDiagnosticTestFailed && shellRequiresWorkspaceBehavior(request) {
		// An assertion's location is a verification input, not proof that its
		// criterion is defective. Only independently observed implementation
		// execution may propose File repair candidates under the default route.
		targetRefs = normalizeShellStrings(output.workspaceBehaviorTargetRefs)
		targetCandidatesValid = len(targetRefs) <= runtimemodel.RuntimeFailureTargetObservationMaxTargetRefs
		if !targetCandidatesValid {
			targetRefs = nil
		}
		// A failing check is a tool fact, not attribution to the deliverable.
		// Do not publish the ordinary implementation-repair route identity when
		// complete, bounded implementation observations are unavailable. This
		// does not establish that the input is wrong or authorize input edits.
		if len(targetRefs) == 0 || output.OutputTruncated {
			ownerCode = "validation_test_failure_unattributed"
			targetType = runtimemodel.ValidationEvidenceTargetValidationOutput
			targetRefs = nil
		}
	}
	if class == runtimemodel.ValidationDiagnosticBuildFailed && len(request.ValidationCheckAttestations) != 0 && (request.ValidationCheckAttestations[0].ObservationProtocolRef == shellGoSupplementObservationProtocolRef || request.ValidationCheckAttestations[0].ObservationProtocolRef == shellGoEntrypointObservationProtocolRef) {
		// A newly added input that does not compile cannot authorize rewriting
		// original acceptance inputs through the ordinary implementation route.
		implementationRefs := make([]string, 0, len(targetRefs))
		for _, ref := range targetRefs {
			if !strings.HasSuffix(ref, "_test.go") && !strings.HasPrefix(ref, "workorch_validation/") {
				implementationRefs = append(implementationRefs, ref)
			}
		}
		targetRefs = implementationRefs
	}
	output.validationFailureTargetCandidatesInvalid = !targetCandidatesValid
	diagnostic := &runtimemodel.ValidationFailureDiagnostic{
		SchemaVersion:                 runtimemodel.ValidationFailureDiagnosticSchemaVersion,
		Class:                         class,
		OwnerCode:                     ownerCode,
		Reason:                        firstNonEmptyShellString(output.Reason, output.FailureReason, errorString(err), output.EvidenceReason),
		RecommendedEvidenceTargetType: targetType,
		Stage:                         firstNonEmptyShellString(output.Stage, "shell_command_failed"),
		Operation:                     shellDiagnosticOperationKind(request),
		CommandKind:                   strings.TrimSpace(filepath.Base(execCommand)),
		ExitCode:                      exitCode, TimedOut: timedOut, CommandPolicyViolation: policyViolation,
		UnavailableCommand: output.UnavailableCommand,
		TargetRefs:         targetRefs,
		ObservationType:    firstNonEmptyShellString(request.ValidationEvidence.ObservationType, request.ValidationEvidence.Observation),
		EvidenceMode:       request.ValidationEvidence.Mode,
		StdoutPreview:      shellPreview(output.Stdout, 2048),
		StderrPreview:      shellPreview(output.Stderr, 2048),
	}
	if ownerCode == "validation_test_failure_unattributed" {
		diagnostic.Reason = "test execution failed, but complete bounded workspace implementation observations are unavailable; neither an implementation defect nor a validation-input defect is established; original failure: " + diagnostic.Reason
	}
	if ownerCode == "validation_workspace_behavior_unobserved" && len(request.ValidationCheckAttestations) == 1 {
		check := request.ValidationCheckAttestations[0]
		if (check.ObservationProtocolRef == shellGoBehaviorObservationProtocolRef || check.ObservationProtocolRef == shellGoEntrypointObservationProtocolRef) && check.SupplementalNativeCheckRef == "" {
			diagnostic.InputAddition = shellValidationInputAddition(check)
		}
	}
	if class == runtimemodel.ValidationDiagnosticBuildFailed {
		diagnostic.InputLocations, diagnostic.InputRepair = shellValidationInputCompileFact(*output, request, execCommand)
		if len(diagnostic.InputLocations) != 0 {
			diagnostic.RecommendedEvidenceTargetType = runtimemodel.ValidationEvidenceTargetValidationInput
			diagnostic.OwnerCode = "validation_input_compile_failed"
			diagnostic.TargetRefs = nil
			location := diagnostic.InputLocations[0]
			diagnostic.Reason = fmt.Sprintf("validation input %s:%d:%d: %s %s", location.TargetRef, location.Line, location.Column, location.Code, location.Symbol)
			if diagnostic.InputRepair != nil {
				diagnostic.OwnerCode = shellInputSymbolRepairCode
				diagnostic.TargetRefs = []string{diagnostic.InputRepair.TargetRef}
			}
		}
	}
	output.ValidationFailureDiagnostic = diagnostic
	output.RuntimeDiagnostics.ValidationFailureDiagnostic = diagnostic
	if class == runtimemodel.ValidationDiagnosticCommandInvalid {
		output.RuntimeDiagnostics.Kind = "shell_command_policy_violation"
		output.RuntimeDiagnostics.Operation = "shell/command"
		output.RuntimeDiagnostics.CommandPolicyViolation = true
	}
}

func shellValidationDiagnosticClassAndTarget(output RuntimeCommandResult, validationEvidence runtimemodel.RuntimeValidationEvidence, execCommand string, timedOut bool, policyViolation bool, err error) (string, string, string) {
	ownerCode := func(class, fallback string) string {
		return shellValidationOwnerDiagnosticCode(output, validationEvidence, execCommand, class, fallback)
	}
	if policyViolation {
		return runtimemodel.ValidationDiagnosticCommandInvalid, runtimemodel.ValidationEvidenceTargetCommandContract, ownerCode(runtimemodel.ValidationDiagnosticCommandInvalid, runtimemodel.ValidationDiagnosticCommandInvalid)
	}
	if timedOut || strings.EqualFold(output.Status, "timeout") {
		return runtimemodel.ValidationDiagnosticTimeout, runtimemodel.ValidationEvidenceTargetValidationOutput, ownerCode(runtimemodel.ValidationDiagnosticTimeout, runtimemodel.ValidationDiagnosticTimeout)
	}
	if output.FailureReason == "command_not_found" || output.CapabilityMissing {
		return runtimemodel.ValidationDiagnosticEnvironmentMissing, runtimemodel.ValidationEvidenceTargetEnvironment, "command_not_found"
	}
	// The managed recipe may carry a generic build/test classification, but a
	// bounded Go tool diagnostic can identify a narrower dependency-contract
	// failure. Preserve that owner fact before consulting the generic profile
	// hint so incidental importing .go locations never become File repair
	// candidates for missing module metadata.
	if output.ExitCode != 0 && shellGoDependencyMetadataMismatch(output, execCommand) {
		return runtimemodel.ValidationDiagnosticDependencyMismatch, runtimemodel.ValidationEvidenceTargetDependencyContract, ownerCode(runtimemodel.ValidationDiagnosticDependencyMismatch, runtimemodel.ValidationDiagnosticDependencyMismatch)
	}
	// A single `go test ./...` process can report package build failures and
	// ordinary test failures together. Build output blocks every affected
	// package from producing trustworthy behavior evidence, so the Shell owner
	// deterministically reports the build surface first. This is a tool-protocol
	// rule local to Shell, not Runtime/profile policy.
	if output.ExitCode != 0 && shellGoBuildFailure(output, execCommand) {
		return runtimemodel.ValidationDiagnosticBuildFailed, runtimemodel.ValidationEvidenceTargetFailureSurface, ownerCode(runtimemodel.ValidationDiagnosticBuildFailed, runtimemodel.ValidationDiagnosticBuildFailed)
	}
	if output.ExitCode != 0 && shellGoTestExecutionFailure(output, execCommand) {
		return runtimemodel.ValidationDiagnosticTestFailed, runtimemodel.ValidationEvidenceTargetBehaviorEvidence, ownerCode(runtimemodel.ValidationDiagnosticTestFailed, runtimemodel.ValidationDiagnosticTestFailed)
	}
	if class := allowedShellValidationDiagnosticClass(firstNonEmptyShellString(
		validationEvidence.DiagnosticClass, validationEvidence.FailureClass, validationEvidence.ExpectedFailureClass,
	)); class != "" {
		return class, firstNonEmptyShellString(validationEvidence.RecommendedEvidenceTargetType, defaultShellValidationDiagnosticTargetType(class)), ownerCode(class, class)
	}
	// Preserve the process failure even if a stale or malformed owner-private
	// result also carries a zero-work marker from an unrelated subpackage.
	if output.ExitCode != 0 {
		switch strings.TrimSpace(firstNonEmptyShellString(validationEvidence.ObservationType, validationEvidence.Observation)) {
		case "dependency_resolution", "dependency_check", "dependency_contract":
			return runtimemodel.ValidationDiagnosticDependencyMismatch, runtimemodel.ValidationEvidenceTargetDependencyContract, ownerCode(runtimemodel.ValidationDiagnosticDependencyMismatch, runtimemodel.ValidationDiagnosticDependencyMismatch)
		case "toolchain_resolution", "toolchain_check", "toolchain_contract":
			return runtimemodel.ValidationDiagnosticToolchainMismatch, runtimemodel.ValidationEvidenceTargetToolchainContract, ownerCode(runtimemodel.ValidationDiagnosticToolchainMismatch, runtimemodel.ValidationDiagnosticToolchainMismatch)
		case "test_result", "assertion_result", "behavior_test_result":
			return runtimemodel.ValidationDiagnosticTestFailed, runtimemodel.ValidationEvidenceTargetBehaviorEvidence, ownerCode(runtimemodel.ValidationDiagnosticTestFailed, runtimemodel.ValidationDiagnosticTestFailed)
		default:
			return runtimemodel.ValidationDiagnosticBuildFailed, runtimemodel.ValidationEvidenceTargetFailureSurface, ownerCode(runtimemodel.ValidationDiagnosticBuildFailed, runtimemodel.ValidationDiagnosticBuildFailed)
		}
	}
	if output.ZeroWork || strings.TrimSpace(output.EvidenceQuality) == string(common.EvidenceQualityInconclusive) {
		return runtimemodel.ValidationDiagnosticOutputInconclusive, runtimemodel.ValidationEvidenceTargetValidationOutput, ownerCode(runtimemodel.ValidationDiagnosticOutputInconclusive, runtimemodel.ValidationDiagnosticOutputInconclusive)
	}
	if err != nil {
		return runtimemodel.ValidationDiagnosticOutputInconclusive, runtimemodel.ValidationEvidenceTargetValidationOutput, ownerCode(runtimemodel.ValidationDiagnosticOutputInconclusive, runtimemodel.ValidationDiagnosticOutputInconclusive)
	}
	return "", "", ""
}

func shellValidationOwnerDiagnosticCode(output RuntimeCommandResult, validationEvidence runtimemodel.RuntimeValidationEvidence, execCommand, class, fallback string) string {
	current := firstNonEmptyShellString(output.FailureCode, output.FailureReason)
	if current != "" && current != "shell_exit_nonzero" {
		return current
	}
	command := strings.TrimSpace(filepath.Base(execCommand))
	if command == "" {
		command = strings.TrimSpace(filepath.Base(output.Command))
	}
	if command != "go" || output.ExitCode == 0 {
		return firstNonEmptyShellString(current, fallback)
	}
	text := shellValidationDiagnosticTextForClass(output, execCommand, class)
	if class == runtimemodel.ValidationDiagnosticBuildFailed && shellGoModuleDiagnosticPattern.MatchString(text) {
		return shellcommon.ValidationDiagnosticCodeModuleManifestParseFailed
	}
	if class == runtimemodel.ValidationDiagnosticBuildFailed &&
		(shellGoDiagnosticTargetPattern.MatchString(text) || shellGoPackageConflictPattern.MatchString(text)) {
		return shellcommon.ValidationDiagnosticCodeSourceCompileFailed
	}
	observation := strings.TrimSpace(firstNonEmptyShellString(validationEvidence.ObservationType, validationEvidence.Observation))
	if class == runtimemodel.ValidationDiagnosticTestFailed || observation == "test_result" || observation == "assertion_result" || observation == "behavior_test_result" {
		return shellcommon.ValidationDiagnosticCodeTestFailed
	}
	return firstNonEmptyShellString(current, fallback)
}

func shellGoDependencyMetadataMismatch(output RuntimeCommandResult, execCommand string) bool {
	command := strings.TrimSpace(filepath.Base(execCommand))
	if command == "" {
		command = strings.TrimSpace(filepath.Base(output.Command))
	}
	if command != "go" {
		return false
	}
	text := strings.ToLower(shellGoBuildDiagnosticOutput(output, execCommand))
	for _, marker := range []string{
		"missing go.sum entry for module providing package",
		"missing go.sum entry needed to verify package",
		"updates to go.mod needed; to update it:",
		"inconsistent vendoring in",
		"no required module provides package",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func shellGoTestExecutionFailure(output RuntimeCommandResult, execCommand string) bool {
	command := strings.TrimSpace(filepath.Base(execCommand))
	if command == "" {
		command = strings.TrimSpace(filepath.Base(output.Command))
	}
	if command != "go" || len(output.Args) == 0 || strings.TrimSpace(output.Args[0]) != "test" {
		return false
	}
	text := shellValidationDiagnosticOutput(output, execCommand)
	return strings.Contains(text, "--- FAIL:")
}

func shellGoBuildFailure(output RuntimeCommandResult, execCommand string) bool {
	command := strings.TrimSpace(filepath.Base(execCommand))
	if command == "" {
		command = strings.TrimSpace(filepath.Base(output.Command))
	}
	if command != "go" {
		return false
	}
	if facts, ok := shellGoTestJSONFactsForOutput(output, execCommand); ok {
		if facts.HasBuildOutput {
			return true
		}
		for _, record := range facts.DiagnosticRecords {
			if record.Action == "" && shellGoDiagnosticChunkIsBuild(record.Output) {
				return true
			}
		}
		return false
	}
	text := shellValidationDiagnosticOutput(output, execCommand)
	if shellGoModuleDiagnosticPattern.MatchString(text) || shellGoPackageConflictPattern.MatchString(text) {
		return true
	}
	for _, line := range strings.Split(text, "\n") {
		if shellGoDiagnosticTargetPattern.MatchString(line) && shellGoCompilerDiagnosticLine(line) {
			return true
		}
	}
	return false
}

func shellGoCompilerDiagnosticLine(line string) bool {
	line = strings.ToLower(strings.TrimSpace(line))
	for _, marker := range []string{
		"undefined:", "syntax error", "cannot use ", "declared and not used", "imported and not used",
		"assignment mismatch", "not enough arguments", "too many arguments", "invalid operation",
		"missing return", "redeclared in this block", "too many errors",
	} {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

// shellValidationDiagnosticOutput is the sole owner-private normalization of
// command output used by diagnostic classification and target extraction. A
// managed `go test -json` transcript contributes only its bounded,
// tool-authored output/build-output records; other commands retain their raw
// bounded stdout/stderr.
func shellValidationDiagnosticOutput(output RuntimeCommandResult, execCommand string) string {
	text := output.Stdout + "\n" + output.Stderr
	command := strings.TrimSpace(filepath.Base(execCommand))
	if command == "" {
		command = strings.TrimSpace(filepath.Base(output.Command))
	}
	if command == "go" && len(output.Args) > 0 && strings.TrimSpace(output.Args[0]) == "test" && shellGoTestJSONEnabled(output.Args[1:]) {
		facts, _ := shellGoTestJSONFactsForOutput(output, execCommand)
		return facts.DiagnosticOutput
	}
	return text
}

func shellGoTestJSONFactsForOutput(output RuntimeCommandResult, execCommand string) (shellGoTestJSONFacts, bool) {
	command := strings.TrimSpace(filepath.Base(execCommand))
	if command == "" {
		command = strings.TrimSpace(filepath.Base(output.Command))
	}
	if command != "go" || len(output.Args) == 0 || strings.TrimSpace(output.Args[0]) != "test" || !shellGoTestJSONEnabled(output.Args[1:]) {
		return shellGoTestJSONFacts{}, false
	}
	if output.goTestJSONFacts != nil {
		return *output.goTestJSONFacts, true
	}
	return parseGoTestJSONFacts(output.Stdout, output.Stderr), true
}

func shellDiagnosticOperationKind(request RuntimeCommandRequest) string {
	if shellInputHasManagedValidation(request) {
		return runtimemodel.RuntimeBlockKindManagedValidationSession
	}
	return runtimemodel.StepKindShellCommand
}

func allowedShellValidationDiagnosticClass(value string) string {
	switch strings.TrimSpace(value) {
	case runtimemodel.ValidationDiagnosticCommandInvalid,
		runtimemodel.ValidationDiagnosticEnvironmentMissing,
		runtimemodel.ValidationDiagnosticDependencyMismatch,
		runtimemodel.ValidationDiagnosticToolchainMismatch,
		runtimemodel.ValidationDiagnosticBuildFailed,
		runtimemodel.ValidationDiagnosticTestFailed,
		runtimemodel.ValidationDiagnosticTimeout,
		runtimemodel.ValidationDiagnosticOutputInconclusive,
		runtimemodel.ValidationDiagnosticEvidenceBindingMissing,
		runtimemodel.ValidationDiagnosticImplementationGap:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func defaultShellValidationDiagnosticTargetType(class string) string {
	switch strings.TrimSpace(class) {
	case runtimemodel.ValidationDiagnosticCommandInvalid:
		return runtimemodel.ValidationEvidenceTargetCommandContract
	case runtimemodel.ValidationDiagnosticEnvironmentMissing:
		return runtimemodel.ValidationEvidenceTargetEnvironment
	case runtimemodel.ValidationDiagnosticDependencyMismatch:
		return runtimemodel.ValidationEvidenceTargetDependencyContract
	case runtimemodel.ValidationDiagnosticToolchainMismatch:
		return runtimemodel.ValidationEvidenceTargetToolchainContract
	case runtimemodel.ValidationDiagnosticEvidenceBindingMissing:
		return runtimemodel.ValidationEvidenceTargetCoverageBinding
	case runtimemodel.ValidationDiagnosticTestFailed:
		return runtimemodel.ValidationEvidenceTargetBehaviorEvidence
	case runtimemodel.ValidationDiagnosticOutputInconclusive, runtimemodel.ValidationDiagnosticTimeout:
		return runtimemodel.ValidationEvidenceTargetValidationOutput
	default:
		return runtimemodel.ValidationEvidenceTargetFailureSurface
	}
}

var (
	shellGoDiagnosticTargetPattern = regexp.MustCompile(`(?:^|[[:space:]])((?:\./)?[[:alnum:]_./-]+\.go):[0-9]+(?::[0-9]+)?:`)
	shellGoModuleDiagnosticPattern = regexp.MustCompile(`(?:^|[[:space:]])((?:\./)?(?:go\.mod|go\.work|go\.sum)):[0-9]+(?::[0-9]+)?:`)
	// go test/go list report a package-name conflict without line/column
	// locations. The two parenthesized file names and the directory are still
	// structured tool output and can be safely intersected with the File-owner
	// manifest scope; arbitrary parenthesized text is intentionally ignored.
	shellGoPackageConflictPattern = regexp.MustCompile(`(?m)^found packages [^()\r\n]+ \(([^()\r\n]+\.go)\) and [^()\r\n]+ \(([^()\r\n]+\.go)\) in ([^\r\n]+)$`)
)

// shellValidationDiagnosticTargetRefs is an owner-private diagnostic adapter.
// It recognizes only bounded Go compiler/test locations from a managed Go
// recipe and intersects them with the fresh File-owner manifest scope carried
// on the typed command. Command targets and validation-entry hints are never
// promoted: when output has no supported location, the result is deliberately
// empty and no repair-surface observation is emitted.
func shellValidationDiagnosticTargetRefs(output RuntimeCommandResult, request RuntimeCommandRequest, execCommand string) []string {
	refs, _ := shellValidationDiagnosticTargetCandidates(output, request, execCommand, "")
	return refs
}

// shellValidationDiagnosticTargetCandidates distinguishes an empty valid
// parser result from a bounded-protocol overflow. The caller carries that bit
// alongside the owner-private command result so a valid marker cannot revive a
// transaction whose structured-output half already exceeded the bound.
func shellValidationDiagnosticTargetCandidates(output RuntimeCommandResult, request RuntimeCommandRequest, execCommand, diagnosticClass string) ([]string, bool) {
	if !shellInputHasManagedValidation(request) || strings.TrimSpace(filepath.Base(execCommand)) != "go" || len(request.ValidationScopeTargetRefs) == 0 {
		return nil, true
	}
	scope := make(map[string]struct{}, len(request.ValidationScopeTargetRefs))
	for _, value := range request.ValidationScopeTargetRefs {
		if normalized, ok := normalizeShellDiagnosticTargetRef(value, output.WorkingDirectory); ok {
			scope[normalized] = struct{}{}
		}
	}
	if len(scope) == 0 {
		return nil, true
	}
	seen := make(map[string]struct{})
	for _, record := range shellValidationDiagnosticRecords(output, execCommand, diagnosticClass) {
		for _, pattern := range []*regexp.Regexp{shellGoDiagnosticTargetPattern, shellGoModuleDiagnosticPattern} {
			for _, match := range pattern.FindAllStringSubmatch(record.Output, -1) {
				if len(match) != 2 {
					continue
				}
				value, ok := shellDiagnosticTargetInScope(match[1], record.Package, record.ImportPath, output.WorkingDirectory, scope)
				if !ok {
					continue
				}
				seen[value] = struct{}{}
				if len(seen) > shellValidationDiagnosticMaxTargetRefs {
					return nil, false
				}
			}
		}
		for _, match := range shellGoPackageConflictPattern.FindAllStringSubmatch(record.Output, -1) {
			if len(match) != 4 {
				continue
			}
			directory := strings.TrimSpace(match[3])
			for _, fileName := range match[1:3] {
				candidate := strings.TrimSpace(fileName)
				if directory != "" {
					candidate = filepath.Join(directory, candidate)
				}
				value, ok := shellDiagnosticTargetInScope(candidate, record.Package, record.ImportPath, output.WorkingDirectory, scope)
				if !ok {
					continue
				}
				seen[value] = struct{}{}
				if len(seen) > shellValidationDiagnosticMaxTargetRefs {
					return nil, false
				}
			}
		}
	}
	ret := make([]string, 0, len(seen))
	for value := range seen {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret, true
}

func shellValidationDiagnosticRecords(output RuntimeCommandResult, execCommand, diagnosticClass string) []shellGoTestJSONDiagnosticRecord {
	if facts, ok := shellGoTestJSONFactsForOutput(output, execCommand); ok {
		ret := make([]shellGoTestJSONDiagnosticRecord, 0, len(facts.DiagnosticRecords))
		for _, record := range facts.DiagnosticRecords {
			switch strings.TrimSpace(diagnosticClass) {
			case runtimemodel.ValidationDiagnosticBuildFailed:
				// Output records belong to the test process and can contain arbitrary
				// text. Only the Go tool's build-output action (plus bounded
				// unframed stderr) may identify a build repair surface.
				if record.Action != "build-output" && record.Action != "" {
					continue
				}
			case runtimemodel.ValidationDiagnosticTestFailed:
				if record.Action == "build-output" {
					continue
				}
			}
			ret = append(ret, record)
		}
		return ret
	}
	return []shellGoTestJSONDiagnosticRecord{{Output: shellValidationDiagnosticOutput(output, execCommand)}}
}

func shellValidationDiagnosticTextForClass(output RuntimeCommandResult, execCommand, diagnosticClass string) string {
	var text strings.Builder
	for _, record := range shellValidationDiagnosticRecords(output, execCommand, diagnosticClass) {
		text.WriteString(record.Output)
		if !strings.HasSuffix(record.Output, "\n") {
			text.WriteByte('\n')
		}
	}
	return text.String()
}

func shellGoBuildDiagnosticOutput(output RuntimeCommandResult, execCommand string) string {
	return shellValidationDiagnosticTextForClass(output, execCommand, runtimemodel.ValidationDiagnosticBuildFailed)
}

func shellGoDiagnosticChunkIsBuild(value string) bool {
	if shellGoModuleDiagnosticPattern.MatchString(value) || shellGoPackageConflictPattern.MatchString(value) {
		return true
	}
	for _, line := range strings.Split(value, "\n") {
		if shellGoDiagnosticTargetPattern.MatchString(line) && shellGoCompilerDiagnosticLine(line) {
			return true
		}
	}
	return false
}

// shellDiagnosticTargetInScope resolves an owner-tool location only against
// the finite File-owner target set sealed into the Shell catalog. Go's JSON
// protocol may report a package-relative file (for example `server_test.go`);
// Package/ImportPath disambiguates that basename without reading go.mod or the
// workspace from another component.
func shellDiagnosticTargetInScope(value, packageRef, importPath, workingDirectory string, scope map[string]struct{}) (string, bool) {
	normalized, ok := normalizeShellDiagnosticTargetRef(value, workingDirectory)
	if !ok {
		return "", false
	}
	if _, allowed := scope[normalized]; allowed {
		return normalized, true
	}
	if filepath.IsAbs(strings.TrimSpace(value)) {
		return "", false
	}
	packageIDs := normalizeShellStrings([]string{packageRef, importPath})
	if len(packageIDs) == 0 {
		return "", false
	}
	matches := make([]string, 0, 1)
	for candidate := range scope {
		if candidate != normalized && !strings.HasSuffix(candidate, "/"+normalized) {
			continue
		}
		directory := path.Dir(candidate)
		if directory == "." || !shellPackageIdentityMatchesDirectory(packageIDs, directory) {
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

func shellPackageIdentityMatchesDirectory(packageIDs []string, directory string) bool {
	directory = strings.Trim(strings.TrimSpace(filepath.ToSlash(directory)), "/")
	if directory == "" || directory == "." {
		return false
	}
	for _, packageID := range packageIDs {
		packageID = strings.Trim(strings.TrimSpace(filepath.ToSlash(packageID)), "/")
		if packageID == directory || strings.HasSuffix(packageID, "/"+directory) {
			return true
		}
	}
	return false
}

func normalizeShellDiagnosticTargetRef(value, workingDirectory string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if filepath.IsAbs(value) {
		workingDirectory = strings.TrimSpace(workingDirectory)
		if workingDirectory == "" {
			return "", false
		}
		root, err := filepath.Abs(workingDirectory)
		if err != nil || root == "" {
			return "", false
		}
		relative, err := filepath.Rel(root, value)
		if err != nil {
			return "", false
		}
		value = relative
	}
	value = filepath.Clean(strings.TrimPrefix(value, "./"))
	if value == "." || value == "" || filepath.IsAbs(value) || value == ".." || strings.HasPrefix(value, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(value), true
}

func normalizeShellStrings(values []string) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			ret = append(ret, value)
		}
	}
	return ret
}

func cloneRuntimeValidationEvidence(value runtimemodel.RuntimeValidationEvidence) runtimemodel.RuntimeValidationEvidence {
	value.TargetRefs = append([]string(nil), value.TargetRefs...)
	value.CoverageTargets = append([]string(nil), value.CoverageTargets...)
	value.ObservedClaims = append([]string(nil), value.ObservedClaims...)
	return value
}

func firstNonEmptyShellString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func shellPreview(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func shellCommandNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	var execErr *exec.Error
	return errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound)
}

func finalizeShellResult(output RuntimeCommandResult, executeErr error) (RuntimeCommandResult, error) {
	output.FailureCode = firstNonEmptyShellString(output.FailureCode, output.FailureReason, output.Stage)
	baseMessage := firstNonEmptyShellString(output.FailureMessage, output.Reason, errorString(executeErr), output.EvidenceReason)
	if output.ValidationFailureDiagnostic != nil && len(output.ValidationFailureDiagnostic.InputLocations) != 0 {
		baseMessage = output.ValidationFailureDiagnostic.Reason
	}
	output.FailureMessage = enrichShellFailureMessage(baseMessage, output)
	summary := firstNonEmptyShellString(output.Summary, output.Status)
	if summary == "" && output.Command != "" {
		summary = fmt.Sprintf("shell command %s", output.Command)
	}
	// On failure, prefer a diagnostic summary so RuntimeEvidence carries repairable signal.
	if executeErr != nil || output.Status == "failed" || output.Status == "timeout" || output.ExitCode != 0 {
		summary = firstNonEmptyShellString(output.FailureMessage, summary)
	}
	output.Summary = summary
	exitCode := output.ExitCode
	output.Evidence = runtimemodel.RuntimeOperationEvidence{
		Summary: summary, ExitCode: &exitCode,
		Quality: common.EvidenceQuality(strings.TrimSpace(output.EvidenceQuality)), ZeroWork: output.ZeroWork,
		EvidenceReason: output.EvidenceReason, EvidenceTags: append([]string(nil), output.EvidenceTags...),
	}
	if output.ValidationObservation != nil {
		observation := output.ValidationObservation.Clone()
		output.Evidence.Validation = &observation
	}
	return output, executeErr
}

func enrichShellFailureMessage(base string, output RuntimeCommandResult) string {
	base = strings.TrimSpace(base)
	// Redact before preview so secrets never enter FailureMessage / RuntimeFacts / LLM context.
	stderrPreview := shellPreview(common.RedactSensitiveString(output.Stderr), 512)
	stdoutPreview := shellPreview(common.RedactSensitiveString(output.Stdout), 256)
	base = common.RedactSensitiveString(base)
	parts := make([]string, 0, 4)
	if base != "" {
		parts = append(parts, base)
	}
	if output.ExitCode != 0 && !strings.Contains(base, fmt.Sprintf("exit status %d", output.ExitCode)) && !strings.Contains(base, fmt.Sprintf("exit code %d", output.ExitCode)) {
		parts = append(parts, fmt.Sprintf("exitCode=%d", output.ExitCode))
	}
	if stage := strings.TrimSpace(output.Stage); stage != "" && !strings.Contains(base, stage) {
		parts = append(parts, "stage="+stage)
	}
	if stderrPreview != "" {
		parts = append(parts, "stderr="+stderrPreview)
	} else if stdoutPreview != "" {
		parts = append(parts, "stdout="+stdoutPreview)
	}
	return strings.Join(parts, "; ")
}
