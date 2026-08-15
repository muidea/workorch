package shell

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

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
	class, targetType := shellValidationDiagnosticClassAndTarget(*output, request.ValidationEvidence, timedOut, policyViolation, err)
	if class == "" {
		return
	}
	diagnostic := &runtimemodel.ValidationFailureDiagnostic{
		SchemaVersion:                 runtimemodel.ValidationFailureDiagnosticSchemaVersion,
		Class:                         class,
		Reason:                        firstNonEmptyShellString(output.Reason, output.FailureReason, errorString(err), output.EvidenceReason),
		RecommendedEvidenceTargetType: targetType,
		Stage:                         firstNonEmptyShellString(output.Stage, "shell_command_failed"),
		Operation:                     "shell/command",
		CommandKind:                   strings.TrimSpace(filepath.Base(execCommand)),
		ExitCode:                      exitCode, TimedOut: timedOut, CommandPolicyViolation: policyViolation,
		UnavailableCommand: output.UnavailableCommand,
		TargetRefs:         shellValidationDiagnosticTargetRefs(*output, request.ValidationEvidence),
		ObservationType:    firstNonEmptyShellString(request.ValidationEvidence.ObservationType, request.ValidationEvidence.Observation),
		EvidenceMode:       request.ValidationEvidence.Mode,
		StdoutPreview:      shellPreview(output.Stdout, 2048),
		StderrPreview:      shellPreview(output.Stderr, 2048),
	}
	output.ValidationFailureDiagnostic = diagnostic
	output.RuntimeDiagnostics.ValidationFailureDiagnostic = diagnostic
	if class == runtimemodel.ValidationDiagnosticCommandInvalid {
		output.RuntimeDiagnostics.Kind = "shell_command_policy_violation"
		output.RuntimeDiagnostics.Operation = "shell/command"
		output.RuntimeDiagnostics.CommandPolicyViolation = true
	}
}

func shellValidationDiagnosticClassAndTarget(output RuntimeCommandResult, validationEvidence runtimemodel.RuntimeValidationEvidence, timedOut bool, policyViolation bool, err error) (string, string) {
	if policyViolation {
		return runtimemodel.ValidationDiagnosticCommandInvalid, runtimemodel.ValidationEvidenceTargetCommandContract
	}
	if timedOut || strings.EqualFold(output.Status, "timeout") {
		return runtimemodel.ValidationDiagnosticTimeout, runtimemodel.ValidationEvidenceTargetValidationOutput
	}
	if output.FailureReason == "command_not_found" || output.CapabilityMissing {
		return runtimemodel.ValidationDiagnosticEnvironmentMissing, runtimemodel.ValidationEvidenceTargetEnvironment
	}
	if class := allowedShellValidationDiagnosticClass(firstNonEmptyShellString(
		validationEvidence.DiagnosticClass, validationEvidence.FailureClass, validationEvidence.ExpectedFailureClass,
	)); class != "" {
		return class, firstNonEmptyShellString(validationEvidence.RecommendedEvidenceTargetType, defaultShellValidationDiagnosticTargetType(class))
	}
	if output.ZeroWork || strings.TrimSpace(output.EvidenceQuality) == string(common.EvidenceQualityInconclusive) {
		return runtimemodel.ValidationDiagnosticOutputInconclusive, runtimemodel.ValidationEvidenceTargetValidationOutput
	}
	if output.ExitCode != 0 {
		switch strings.TrimSpace(firstNonEmptyShellString(validationEvidence.ObservationType, validationEvidence.Observation)) {
		case "dependency_resolution", "dependency_check", "dependency_contract":
			return runtimemodel.ValidationDiagnosticDependencyMismatch, runtimemodel.ValidationEvidenceTargetDependencyContract
		case "toolchain_resolution", "toolchain_check", "toolchain_contract":
			return runtimemodel.ValidationDiagnosticToolchainMismatch, runtimemodel.ValidationEvidenceTargetToolchainContract
		case "test_result", "assertion_result", "behavior_test_result":
			return runtimemodel.ValidationDiagnosticTestFailed, runtimemodel.ValidationEvidenceTargetBehaviorEvidence
		default:
			return runtimemodel.ValidationDiagnosticBuildFailed, runtimemodel.ValidationEvidenceTargetFailureSurface
		}
	}
	if err != nil {
		return runtimemodel.ValidationDiagnosticOutputInconclusive, runtimemodel.ValidationEvidenceTargetValidationOutput
	}
	return "", ""
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
	case runtimemodel.ValidationDiagnosticOutputInconclusive, runtimemodel.ValidationDiagnosticTimeout:
		return runtimemodel.ValidationEvidenceTargetValidationOutput
	default:
		return runtimemodel.ValidationEvidenceTargetFailureSurface
	}
}

func shellValidationDiagnosticTargetRefs(output RuntimeCommandResult, validationEvidence runtimemodel.RuntimeValidationEvidence) []string {
	seen := map[string]bool{}
	ret := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, value := range output.TargetRefs {
		add(value)
	}
	for _, value := range validationEvidence.TargetRefs {
		add(value)
	}
	for _, value := range validationEvidence.CoverageTargets {
		add(value)
	}
	return ret
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
