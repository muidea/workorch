package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	DefaultCommandTimeout = 60 * time.Second
	MaxCommandTimeout     = 10 * time.Minute
	MaxCommandOutputBytes = 64 * 1024
)

type RuntimeAdapter struct {
	timeout      time.Duration
	envAllowlist map[string]struct{}
}

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{
		timeout: DefaultCommandTimeout,
		envAllowlist: map[string]struct{}{
			"HOME":   {},
			"PATH":   {},
			"TMPDIR": {},
		},
	}
}

func NewRuntimeAdapterWithTimeout(timeout time.Duration) *RuntimeAdapter {
	adapter := NewRuntimeAdapter()
	adapter.timeout = timeout
	return adapter
}

func NewRuntimeAdapterFromTimeoutString(timeoutValue string) (*RuntimeAdapter, error) {
	adapter := NewRuntimeAdapter()
	timeoutValue = strings.TrimSpace(timeoutValue)
	if timeoutValue == "" {
		return adapter, nil
	}
	timeout, err := time.ParseDuration(timeoutValue)
	if err != nil {
		return nil, fmt.Errorf("%w: runtime.shellCommandTimeout must be a valid duration", common.ErrInvalidArgument)
	}
	if timeout < time.Second {
		return nil, fmt.Errorf("%w: runtime.shellCommandTimeout must be at least 1s", common.ErrInvalidArgument)
	}
	if timeout > MaxCommandTimeout {
		return nil, fmt.Errorf("%w: runtime.shellCommandTimeout must not exceed %s", common.ErrInvalidArgument, MaxCommandTimeout)
	}
	adapter.timeout = timeout
	return adapter, nil
}

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	command, args, err := shellCommandSpec(request.Input)
	if err != nil {
		return runtimemodel.StepExecutionResult{}, err
	}
	deliverableDirectory, err := filepath.Abs(strings.TrimSpace(request.WorkspaceDeliverableDirectory))
	if err != nil || deliverableDirectory == "" {
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
	if err := os.MkdirAll(deliverableDirectory, 0o755); err != nil {
		return runtimemodel.StepExecutionResult{}, err
	}
	execCommand, execArgs, commandNormalization := normalizeShellExecutionCommand(command, args)
	if err := validateShellCommandPolicy(execCommand, execArgs, deliverableDirectory, shellInputHasManagedValidation(request.Input)); err != nil {
		now := time.Now().UTC()
		output := shellBaseOutput(request.Input, command, args, deliverableDirectory, commandNormalization, execCommand)
		output["status"] = "failed"
		output["stage"] = "shell_command_policy_rejected"
		output["reason"] = err.Error()
		attachManagedValidationResources(output, request, 0, now, now, "released")
		attachShellValidationFailureDiagnostic(output, request.Input, execCommand, 0, false, true, err)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	if err := validateValidationShellEvidencePolicy(request.Input, execCommand, execArgs); err != nil {
		now := time.Now().UTC()
		output := shellBaseOutput(request.Input, command, args, deliverableDirectory, commandNormalization, execCommand)
		output["status"] = "failed"
		output["stage"] = "shell_validation_contract_rejected"
		output["reason"] = err.Error()
		output["failureReason"] = "validation_shell_masks_failure"
		attachManagedValidationResources(output, request, 0, now, now, "released")
		attachShellValidationFailureDiagnostic(output, request.Input, execCommand, 0, false, true, err)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}

	timeout, err := shellCommandTimeout(request.Input, a.timeout)
	if err != nil {
		now := time.Now().UTC()
		output := shellBaseOutput(request.Input, command, args, deliverableDirectory, commandNormalization, execCommand)
		output["status"] = "failed"
		output["stage"] = "shell_command_timeout_policy_rejected"
		output["reason"] = err.Error()
		attachManagedValidationResources(output, request, 0, now, now, "released")
		attachShellValidationFailureDiagnostic(output, request.Input, execCommand, 0, false, true, err)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	beforeChange := workspaceStateChangeFingerprint(ctx, deliverableDirectory, boolFromShellValue(request.Input["stateChanging"]))
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startedAt := time.Now().UTC()
	cmd := exec.CommandContext(runCtx, execCommand, execArgs...)
	cmd.Dir = deliverableDirectory
	cmd.Env = a.filteredEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
	stdout := newLimitedOutputBuffer(MaxCommandOutputBytes)
	stderr := newLimitedOutputBuffer(MaxCommandOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	pid := 0
	err = cmd.Start()
	if err == nil {
		if cmd.Process != nil {
			pid = cmd.Process.Pid
		}
		err = cmd.Wait()
	}
	duration := time.Since(startedAt)
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	stdoutText := stdout.String()
	stderrText := stderr.String()
	output := shellBaseOutput(request.Input, command, args, deliverableDirectory, commandNormalization, execCommand)
	output["status"] = "completed"
	output["stdout"] = stdoutText
	output["stderr"] = sanitizeSuccessfulCommandStderr(command, args, exitCode, stdoutText, stderrText)
	output["exitCode"] = exitCode
	output["durationMillis"] = duration.Milliseconds()
	output["timeoutMillis"] = timeout.Milliseconds()
	if stdout.Truncated() || stderr.Truncated() {
		output["outputTruncated"] = stdout.Truncated() || stderr.Truncated()
		output["stdoutTruncated"] = stdout.Truncated()
		output["stderrTruncated"] = stderr.Truncated()
		output["stdoutBytes"] = stdout.TotalBytes()
		output["stderrBytes"] = stderr.TotalBytes()
		output["maxOutputBytes"] = MaxCommandOutputBytes
	}
	applyShellEvidencePolicies(request.EvidencePolicies, command, args, output)
	if runCtx.Err() == context.DeadlineExceeded {
		output["status"] = "timeout"
		output["stage"] = "shell_command_timeout"
		attachManagedValidationResources(output, request, pid, startedAt, time.Now().UTC(), "timeout")
		attachShellValidationFailureDiagnostic(output, request.Input, execCommand, exitCode, true, false, context.DeadlineExceeded)
		return runtimemodel.StepExecutionResult{Output: output}, context.DeadlineExceeded
	}
	if err != nil {
		output["status"] = "failed"
		attachManagedValidationResources(output, request, pid, startedAt, time.Now().UTC(), "failed")
		attachShellCommandFailureDiagnostics(output, execCommand, err)
		attachShellValidationFailureDiagnostic(output, request.Input, execCommand, exitCode, false, false, err)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	if beforeChange.Available {
		afterChange := workspaceStateChangeFingerprint(ctx, deliverableDirectory, true)
		if afterChange.Available {
			output["workspaceStateChangeFingerprint"] = map[string]any{
				"before": beforeChange.Value,
				"after":  afterChange.Value,
			}
			if beforeChange.Value == afterChange.Value {
				err := fmt.Errorf("%w: state-changing shell command produced no observable workspace delta", common.ErrInvalidArgument)
				output["status"] = "failed"
				output["stage"] = "shell_state_changing_no_progress"
				output["failureReason"] = "state_changing_no_workspace_delta"
				output["zeroWork"] = true
				output["evidenceQuality"] = string(common.EvidenceQualityInconclusive)
				output["evidenceReason"] = "state-changing shell command completed without changing tracked workspace state, untracked files, or HEAD"
				output["runtimeDiagnostics"] = map[string]any{
					"kind":          "state_changing_no_progress",
					"operation":     "shell/command",
					"failureReason": "state_changing_no_workspace_delta",
					"targetRef":     stringFromShellValue(request.Input["targetRef"]),
				}
				attachManagedValidationResources(output, request, pid, startedAt, time.Now().UTC(), "failed")
				attachShellValidationFailureDiagnostic(output, request.Input, execCommand, exitCode, false, false, err)
				return runtimemodel.StepExecutionResult{Output: output}, err
			}
		}
	}
	attachManagedValidationResources(output, request, pid, startedAt, time.Now().UTC(), "released")
	return runtimemodel.StepExecutionResult{Output: output}, nil
}

type shellWorkspaceChangeFingerprint struct {
	Available bool
	Value     string
}

func workspaceStateChangeFingerprint(ctx context.Context, deliverableDirectory string, required bool) shellWorkspaceChangeFingerprint {
	if !required {
		return shellWorkspaceChangeFingerprint{}
	}
	dir := strings.TrimSpace(deliverableDirectory)
	if dir == "" {
		return shellWorkspaceChangeFingerprint{}
	}
	gitDir := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return shellWorkspaceChangeFingerprint{}
	}
	snapshotCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	head := strings.TrimSpace(gitCommandOutput(snapshotCtx, dir, "rev-parse", "--verify", "HEAD"))
	status := strings.TrimSpace(gitCommandOutput(snapshotCtx, dir, "status", "--porcelain=v1", "--untracked-files=all"))
	return shellWorkspaceChangeFingerprint{
		Available: true,
		Value:     "head=" + head + "\nstatus=" + status,
	}
}

func gitCommandOutput(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func shellBaseOutput(input map[string]any, command string, args []string, workingDir string, commandNormalization string, execCommand string) map[string]any {
	output := map[string]any{
		"command":    command,
		"args":       args,
		"workingDir": workingDir,
	}
	if commandNormalization != "" {
		output["actualCommand"] = filepath.Base(execCommand)
		output["commandNormalization"] = commandNormalization
	}
	if evidence := mapFromShellInput(input, "validationEvidence"); len(evidence) > 0 {
		output["validationEvidence"] = evidence
	}
	if managed := mapFromShellInput(input, "managedValidation"); len(managed) > 0 {
		output["managedValidation"] = managed
	}
	if refs := shellStringSliceFromValue(input["targetRefs"]); len(refs) > 0 {
		output["targetRefs"] = refs
	}
	return output
}

func attachManagedValidationResources(output map[string]any, request runtimemodel.StepExecutionRequest, pid int, startedAt time.Time, finishedAt time.Time, processStatus string) {
	if output == nil || !shellInputHasValidationResourceLifecycle(request.Input) {
		return
	}
	managedInput := shellInputHasManagedValidation(request.Input)
	resources := make([]map[string]any, 0, 2)
	if pid > 0 {
		resources = append(resources, map[string]any{
			"id":            shellManagedValidationResourceID(request, "process"),
			"workspaceId":   string(request.WorkspaceID),
			"runId":         string(request.RunID),
			"stepId":        string(request.StepID),
			"kind":          "process",
			"pid":           pid,
			"status":        processStatus,
			"cleanupPolicy": "process_group_on_timeout_or_completion",
			"createdAt":     startedAt.Format(time.RFC3339Nano),
			"releasedAt":    finishedAt.Format(time.RFC3339Nano),
		})
	}
	if scratch := strings.TrimSpace(stringFromShellValue(request.Input["validationScratchDir"])); scratch != "" {
		autoCleanup := managedValidationScratchAutoCleanupEnabled(request.Input)
		cleanupPolicy := "remove_step_scoped_validation_scratch_on_completion"
		status := "released"
		if !autoCleanup {
			cleanupPolicy = "retain_validation_scratch_until_run_terminal"
			status = "retained"
		}
		record := map[string]any{
			"id":            shellManagedValidationResourceID(request, "scratch_dir"),
			"workspaceId":   string(request.WorkspaceID),
			"runId":         string(request.RunID),
			"stepId":        string(request.StepID),
			"kind":          "scratch_dir",
			"path":          scratch,
			"status":        status,
			"cleanupPolicy": cleanupPolicy,
			"createdAt":     startedAt.Format(time.RFC3339Nano),
			"releasedAt":    finishedAt.Format(time.RFC3339Nano),
		}
		if cleanupErr := cleanupManagedValidationScratch(request.Input, request.WorkspaceDeliverableDirectory); cleanupErr != nil {
			record["status"] = "cleanup_failed"
			record["cleanupError"] = cleanupErr.Error()
		}
		resources = append(resources, record)
	}
	if len(resources) == 0 {
		return
	}
	output["validationResources"] = resources
	released := managedValidationResourcesReleased(resources)
	if managedInput {
		managed := mapFromShellOutput(output, "managedValidation")
		if managed == nil {
			managed = map[string]any{}
		}
		managed["resourceCount"] = len(resources)
		managed["resourcesReleased"] = released
		output["managedValidation"] = managed
	}
	runtimeDiagnostics := mapFromShellOutput(output, "runtimeDiagnostics")
	if runtimeDiagnostics == nil {
		runtimeDiagnostics = map[string]any{}
	}
	runtimeDiagnostics["validationResourceLifecycle"] = map[string]any{
		"resourceCount": len(resources),
		"released":      released,
	}
	output["runtimeDiagnostics"] = runtimeDiagnostics
}

func shellInputHasValidationResourceLifecycle(input map[string]any) bool {
	return shellInputHasManagedValidation(input) ||
		strings.TrimSpace(stringFromShellValue(input["validationScratchDir"])) != ""
}

func shellInputHasManagedValidation(input map[string]any) bool {
	managed := mapFromShellInput(input, "managedValidation")
	return boolFromShellValue(managed["runtimeManaged"]) ||
		stringFromShellValue(managed["kind"]) == "validation/managed_session" ||
		stringFromShellValue(managed["originalNodeKind"]) == "validation/managed_session"
}

func shellManagedValidationResourceID(request runtimemodel.StepExecutionRequest, kind string) string {
	parts := []string{
		string(request.WorkspaceID),
		string(request.RunID),
		string(request.StepID),
		strings.TrimSpace(kind),
	}
	return strings.Join(parts, ":")
}

func managedValidationResourcesReleased(resources []map[string]any) bool {
	for _, resource := range resources {
		switch stringFromShellValue(resource["status"]) {
		case "released", "timeout", "failed":
		default:
			return false
		}
	}
	return true
}

func cleanupManagedValidationScratch(input map[string]any, deliverableDirectory string) error {
	if !managedValidationScratchAutoCleanupEnabled(input) {
		return nil
	}
	scratch := strings.TrimSpace(stringFromShellValue(input["validationScratchDir"]))
	if scratch == "" {
		return nil
	}
	scratchAbs, err := filepath.Abs(scratch)
	if err != nil {
		return err
	}
	deliverableAbs, err := filepath.Abs(strings.TrimSpace(deliverableDirectory))
	if err != nil {
		return err
	}
	if scratchAbs == deliverableAbs || strings.HasPrefix(scratchAbs, deliverableAbs+string(os.PathSeparator)) {
		return fmt.Errorf("%w: managed validation scratch cleanup must not target workspace deliverable directory", common.ErrInvalidArgument)
	}
	tempRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return err
	}
	if scratchAbs == tempRoot || !strings.HasPrefix(scratchAbs, tempRoot+string(os.PathSeparator)) {
		return fmt.Errorf("%w: managed validation scratch cleanup must stay under temporary directory", common.ErrInvalidArgument)
	}
	if filepath.Base(scratchAbs) == "" || scratchAbs == string(os.PathSeparator) {
		return fmt.Errorf("%w: managed validation scratch cleanup target is invalid", common.ErrInvalidArgument)
	}
	return os.RemoveAll(scratchAbs)
}

func managedValidationScratchAutoCleanupEnabled(input map[string]any) bool {
	managed := mapFromShellInput(input, "managedValidation")
	if _, ok := managed["autoCleanup"]; ok {
		return boolFromShellValue(managed["autoCleanup"])
	}
	if _, ok := input["validationScratchAutoCleanup"]; ok {
		return boolFromShellValue(input["validationScratchAutoCleanup"])
	}
	return strings.TrimSpace(stringFromShellValue(input["validationScratchDir"])) != ""
}

func attachShellCommandFailureDiagnostics(output map[string]any, execCommand string, err error) {
	if output == nil || err == nil {
		return
	}
	if !shellCommandNotFound(err) {
		return
	}
	command := strings.TrimSpace(filepath.Base(execCommand))
	if command == "" {
		command = strings.TrimSpace(execCommand)
	}
	output["failureReason"] = "command_not_found"
	output["unavailableCommand"] = command
	output["capabilityUnavailable"] = true
	output["runtimeDiagnostics"] = map[string]any{
		"kind":               "runtime_capability_unavailable",
		"failureReason":      "command_not_found",
		"unavailableCommand": command,
	}
}

func attachShellValidationFailureDiagnostic(output map[string]any, input map[string]any, execCommand string, exitCode int, timedOut bool, policyViolation bool, err error) {
	if output == nil {
		return
	}
	validationEvidence := mapFromShellInput(input, "validationEvidence")
	class, targetType := shellValidationDiagnosticClassAndTarget(output, validationEvidence, timedOut, policyViolation, err)
	if class == "" {
		return
	}
	diagnostic := runtimemodel.ValidationFailureDiagnostic{
		SchemaVersion:                 runtimemodel.ValidationFailureDiagnosticSchemaVersion,
		Class:                         class,
		Reason:                        firstNonEmptyShellString(stringFromShellValue(output["reason"]), stringFromShellValue(output["failureReason"]), errorString(err), stringFromShellValue(output["evidenceReason"])),
		RecommendedEvidenceTargetType: targetType,
		Stage:                         firstNonEmptyShellString(stringFromShellValue(output["stage"]), "shell_command_failed"),
		Operation:                     "shell/command",
		CommandKind:                   strings.TrimSpace(filepath.Base(execCommand)),
		ExitCode:                      exitCode,
		TimedOut:                      timedOut,
		CommandPolicyViolation:        policyViolation,
		UnavailableCommand:            stringFromShellValue(output["unavailableCommand"]),
		TargetRefs:                    shellValidationDiagnosticTargetRefs(output, validationEvidence),
		ObservationType:               firstNonEmptyShellString(stringFromShellValue(validationEvidence["observationType"]), stringFromShellValue(validationEvidence["observation"])),
		EvidenceMode:                  stringFromShellValue(validationEvidence["mode"]),
		StdoutPreview:                 shellPreview(stringFromShellValue(output["stdout"]), 2048),
		StderrPreview:                 shellPreview(stringFromShellValue(output["stderr"]), 2048),
	}
	diagnosticMap := diagnostic.ToMap()
	output["validationFailureDiagnostic"] = diagnosticMap
	runtimeDiagnostics := mapFromShellOutput(output, "runtimeDiagnostics")
	if runtimeDiagnostics == nil {
		runtimeDiagnostics = map[string]any{}
	}
	runtimeDiagnostics["validationFailureDiagnostic"] = diagnosticMap
	if class == runtimemodel.ValidationDiagnosticCommandInvalid {
		runtimeDiagnostics["kind"] = "shell_command_policy_violation"
		runtimeDiagnostics["commandPolicyViolation"] = true
	}
	output["runtimeDiagnostics"] = runtimeDiagnostics
}

func shellValidationDiagnosticClassAndTarget(output map[string]any, validationEvidence map[string]any, timedOut bool, policyViolation bool, err error) (string, string) {
	if policyViolation {
		return runtimemodel.ValidationDiagnosticCommandInvalid, runtimemodel.ValidationEvidenceTargetCommandContract
	}
	if timedOut || strings.EqualFold(stringFromShellValue(output["status"]), "timeout") {
		return runtimemodel.ValidationDiagnosticTimeout, runtimemodel.ValidationEvidenceTargetValidationOutput
	}
	if stringFromShellValue(output["failureReason"]) == "command_not_found" || boolFromShellValue(output["capabilityUnavailable"]) {
		return runtimemodel.ValidationDiagnosticEnvironmentMissing, runtimemodel.ValidationEvidenceTargetEnvironment
	}
	if class := allowedShellValidationDiagnosticClass(firstNonEmptyShellString(
		stringFromShellValue(validationEvidence["diagnosticClass"]),
		stringFromShellValue(validationEvidence["failureClass"]),
		stringFromShellValue(validationEvidence["expectedFailureClass"]),
	)); class != "" {
		return class, firstNonEmptyShellString(
			stringFromShellValue(validationEvidence["recommendedEvidenceTargetType"]),
			defaultShellValidationDiagnosticTargetType(class),
		)
	}
	if boolFromShellValue(output["zeroWork"]) || strings.TrimSpace(stringFromShellValue(output["evidenceQuality"])) == string(common.EvidenceQualityInconclusive) {
		return runtimemodel.ValidationDiagnosticOutputInconclusive, runtimemodel.ValidationEvidenceTargetValidationOutput
	}
	if exitCode, ok := numericMillis(output["exitCode"]); ok && exitCode != 0 {
		switch strings.TrimSpace(firstNonEmptyShellString(stringFromShellValue(validationEvidence["observationType"]), stringFromShellValue(validationEvidence["observation"]))) {
		case "dependency_resolution", "dependency_check", "dependency_contract":
			return runtimemodel.ValidationDiagnosticDependencyMismatch, runtimemodel.ValidationEvidenceTargetDependencyContract
		case "toolchain_resolution", "toolchain_check", "toolchain_contract":
			return runtimemodel.ValidationDiagnosticToolchainMismatch, runtimemodel.ValidationEvidenceTargetToolchainContract
		case "test_result", "assertion_result", "behavior_test_result":
			return runtimemodel.ValidationDiagnosticTestFailed, runtimemodel.ValidationEvidenceTargetBehaviorEvidence
		case "build_result", "build_observation", "executable_contract_observation":
			return runtimemodel.ValidationDiagnosticBuildFailed, runtimemodel.ValidationEvidenceTargetFailureSurface
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
		runtimemodel.ValidationDiagnosticEvidenceBindingMissing:
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

func shellValidationDiagnosticTargetRefs(output map[string]any, validationEvidence map[string]any) []string {
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
	for _, value := range shellStringSliceFromValue(output["targetRefs"]) {
		add(value)
	}
	for _, value := range shellStringSliceFromValue(validationEvidence["targetRefs"]) {
		add(value)
	}
	for _, value := range shellStringSliceFromValue(validationEvidence["coverageTargets"]) {
		add(value)
	}
	for _, key := range []string{"targetRef", "path", "file"} {
		add(stringFromShellValue(output[key]))
		add(stringFromShellValue(validationEvidence[key]))
	}
	return ret
}

func mapFromShellInput(input map[string]any, key string) map[string]any {
	if input == nil {
		return nil
	}
	return mapFromShellValue(input[key])
}

func mapFromShellOutput(output map[string]any, key string) map[string]any {
	if output == nil {
		return nil
	}
	return mapFromShellValue(output[key])
}

func mapFromShellValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		ret := map[string]any{}
		for key, item := range typed {
			ret[key] = item
		}
		return ret
	default:
		return nil
	}
}

func shellStringSliceFromValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				ret = append(ret, item)
			}
		}
		return ret
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringFromShellValue(item)); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func stringFromShellValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func boolFromShellValue(value any) bool {
	typed, _ := value.(bool)
	return typed
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

type limitedOutputBuffer struct {
	limit int
	total int
	buf   bytes.Buffer
}

func newLimitedOutputBuffer(limit int) *limitedOutputBuffer {
	if limit <= 0 {
		limit = MaxCommandOutputBytes
	}
	return &limitedOutputBuffer{limit: limit}
}

func (b *limitedOutputBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	written := len(p)
	b.total += len(p)
	if b.buf.Len() >= b.limit {
		return written, nil
	}
	remaining := b.limit - b.buf.Len()
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, _ = b.buf.Write(p)
	return written, nil
}

func (b *limitedOutputBuffer) String() string {
	if b == nil {
		return ""
	}
	return b.buf.String()
}

func (b *limitedOutputBuffer) Truncated() bool {
	return b != nil && b.total > b.limit
}

func (b *limitedOutputBuffer) TotalBytes() int {
	if b == nil {
		return 0
	}
	return b.total
}

func normalizeShellExecutionCommand(command string, args []string) (string, []string, string) {
	if shellCommandRequiresInlineShell(command, args) {
		return "sh", []string{"-c", shellCommandInlineScript(command, args)}, "compound_command_wrapped_with_sh_c"
	}
	bashReason := shellCommandBashNormalizationReason(command, args)
	if bashReason == "" {
		return command, args, ""
	}
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		return command, args, ""
	}
	return bashPath, args, bashReason
}

func shellCommandRequiresInlineShell(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	if base == "" || base == "sh" || base == "bash" || base == "zsh" {
		return false
	}
	if shellCommandIsShellBuiltin(base) && len(args) > 0 {
		return true
	}
	for _, arg := range args {
		if shellCommandArgIsShellOperator(arg) {
			return true
		}
	}
	return false
}

func shellCommandIsShellBuiltin(command string) bool {
	switch strings.TrimSpace(command) {
	case "cd", "export", "alias", "source", ".":
		return true
	default:
		return false
	}
}

func shellCommandArgIsShellOperator(arg string) bool {
	switch strings.TrimSpace(arg) {
	case "&&", "||", ";", "|", ">", ">>", "<", "2>", "2>>":
		return true
	default:
		return false
	}
}

func shellCommandInlineScript(command string, args []string) string {
	parts := []string{shellQuoteCommandToken(command)}
	for _, arg := range args {
		if shellCommandArgIsShellOperator(arg) {
			parts = append(parts, strings.TrimSpace(arg))
			continue
		}
		parts = append(parts, shellQuoteCommandToken(arg))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func shellQuoteCommandToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '_' || r == '-' || r == ':' || r == '=' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func shellCommandBashNormalizationReason(command string, args []string) string {
	if filepath.Base(strings.TrimSpace(command)) != "sh" {
		return ""
	}
	script, ok := inlineShellScript(args)
	if !ok {
		return ""
	}
	normalized := strings.ToLower(script)
	if strings.Contains(normalized, "pipefail") {
		return "sh_script_requires_bash_pipefail"
	}
	if strings.Contains(script, "${PIPESTATUS") || strings.Contains(script, "${PIPESTATUS[") {
		return "sh_script_requires_bash_pipestatus"
	}
	return ""
}

func inlineShellScript(args []string) (string, bool) {
	for idx, arg := range args {
		if arg == "-c" && idx+1 < len(args) {
			return args[idx+1], true
		}
	}
	return "", false
}

func shellCommandTimeout(input map[string]any, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = DefaultCommandTimeout
	}
	if input == nil || input["timeoutMillis"] == nil {
		return defaultTimeout, nil
	}
	value, ok := numericMillis(input["timeoutMillis"])
	if !ok || value <= 0 {
		return 0, common.ErrInvalidArgument
	}
	timeout := time.Duration(value) * time.Millisecond
	if timeout < time.Second {
		return 0, fmt.Errorf("%w: timeoutMillis must be at least 1000", common.ErrInvalidArgument)
	}
	if timeout > MaxCommandTimeout {
		return 0, fmt.Errorf("%w: timeoutMillis must not exceed %d", common.ErrInvalidArgument, MaxCommandTimeout/time.Millisecond)
	}
	return timeout, nil
}

func numericMillis(input any) (int64, bool) {
	switch typed := input.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		if typed != float32(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	default:
		return 0, false
	}
}

func (a *RuntimeAdapter) filteredEnv() []string {
	if a == nil || len(a.envAllowlist) == 0 {
		return nil
	}
	out := make([]string, 0, len(a.envAllowlist))
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, allowed := a.envAllowlist[key]; allowed {
			out = append(out, item)
		}
	}
	return out
}

func shellCommandSpec(input map[string]any) (string, []string, error) {
	if input == nil {
		return "", nil, common.ErrInvalidArgument
	}
	command, _ := input["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, common.ErrInvalidArgument
	}
	args, err := shellCommandArgs(input["args"])
	if err != nil {
		return "", nil, err
	}
	return command, args, nil
}

func shellCommandArgs(input any) ([]string, error) {
	if input == nil {
		return nil, nil
	}
	switch typed := input.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			ret = append(ret, value)
		}
		return ret, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func validateShellCommandPolicy(command string, args []string, deliverableDirectory string, allowManagedProcessControl bool) error {
	base := filepath.Base(strings.TrimSpace(command))
	if base == "" {
		return common.ErrInvalidArgument
	}
	switch base {
	case "sudo", "su", "shutdown", "reboot", "poweroff", "halt", "mkfs", "mount", "umount":
		return fmt.Errorf("%w: shell command %q is not allowed", common.ErrInvalidArgument, base)
	case "kill", "pkill", "killall":
		return fmt.Errorf("%w: process termination command %q is not allowed", common.ErrInvalidArgument, base)
	case "fuser":
		if fuserCommandIsDangerous(args) {
			return fmt.Errorf("%w: process termination command %q is not allowed", common.ErrInvalidArgument, base)
		}
	case "rm":
		if rmCommandIsDangerous(args, deliverableDirectory) {
			return fmt.Errorf("%w: destructive rm command is not allowed", common.ErrInvalidArgument)
		}
	case "chmod":
		if chmodCommandIsDangerous(args) {
			return fmt.Errorf("%w: chmod 777 is not allowed", common.ErrInvalidArgument)
		}
	case "git":
		if gitCommandIsDangerous(args) {
			return fmt.Errorf("%w: git force push is not allowed", common.ErrInvalidArgument)
		}
	case "sh", "bash", "zsh":
		if shellScriptIsDangerous(args, allowManagedProcessControl) {
			return fmt.Errorf("%w: destructive shell script is not allowed", common.ErrInvalidArgument)
		}
		if shellScriptStartsDetachedProcess(args) {
			return fmt.Errorf("%w: detached or background shell script is not allowed; use a bounded foreground command or wait for all background work", common.ErrInvalidArgument)
		}
	}
	return nil
}

func validateValidationShellEvidencePolicy(input map[string]any, command string, args []string) error {
	if len(mapFromShellInput(input, "validationEvidence")) == 0 {
		return nil
	}
	base := filepath.Base(strings.TrimSpace(command))
	if base != "sh" && base != "bash" && base != "zsh" {
		return nil
	}
	script, ok := shellScriptArg(args)
	if !ok {
		return nil
	}
	if shellScriptMasksFailureWithFallback(script) {
		return fmt.Errorf("%w: validation shell command masks command failure with fallback; use fail-fast shell or exit with the failed status", common.ErrInvalidArgument)
	}
	if !shellScriptCapturesExitStatus(script) {
		return nil
	}
	if shellScriptReturnsCapturedExitStatus(script) {
		return nil
	}
	return fmt.Errorf("%w: validation shell command captures exit status without returning it; use fail-fast shell or exit with the captured status", common.ErrInvalidArgument)
}

func shellScriptArg(args []string) (string, bool) {
	for idx, arg := range args {
		if strings.TrimSpace(arg) == "-c" && idx+1 < len(args) {
			return args[idx+1], true
		}
	}
	return "", false
}

func shellScriptMasksFailureWithFallback(script string) bool {
	lower := strings.ToLower(strings.TrimSpace(script))
	for {
		idx := strings.Index(lower, "||")
		if idx < 0 {
			return false
		}
		tail := strings.TrimSpace(lower[idx+2:])
		if shellFailureFallbackMasksStatus(tail) {
			return true
		}
		lower = tail
	}
}

func shellFailureFallbackMasksStatus(tail string) bool {
	segment := shellFallbackSegment(tail)
	if shellSegmentReturnsFailure(segment) {
		return false
	}
	switch shellFirstCommandWord(tail) {
	case ":", "echo", "printf", "true":
		return true
	default:
		return false
	}
}

func shellFallbackSegment(tail string) string {
	segment := strings.TrimSpace(tail)
	if idx := strings.Index(segment, "||"); idx >= 0 {
		segment = segment[:idx]
	}
	return segment
}

func shellSegmentReturnsFailure(segment string) bool {
	for _, marker := range []string{
		"exit ",
		"exit\t",
		"exit\n",
		"return ",
		"return\t",
		"return\n",
	} {
		if strings.Contains(segment, marker) {
			return true
		}
	}
	return false
}

func shellFirstCommandWord(tail string) string {
	trimmed := strings.TrimSpace(tail)
	for {
		next := strings.TrimLeft(trimmed, "{( ")
		if next == trimmed {
			break
		}
		trimmed = next
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], ";")
}

func shellScriptCapturesExitStatus(script string) bool {
	lower := strings.ToLower(strings.TrimSpace(script))
	return strings.Contains(lower, "$?") ||
		strings.Contains(lower, "exit_code=") ||
		strings.Contains(lower, "exitcode=")
}

func shellScriptReturnsCapturedExitStatus(script string) bool {
	lower := strings.ToLower(strings.TrimSpace(script))
	for _, marker := range []string{
		"exit $?",
		"exit ${?}",
		"exit $status",
		"exit ${status}",
		"exit $exit_code",
		"exit ${exit_code}",
		"exit $exitcode",
		"exit ${exitcode}",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func rmCommandIsDangerous(args []string, deliverableDirectory string) bool {
	recursiveForce := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && strings.Contains(arg, "r") && strings.Contains(arg, "f") {
			recursiveForce = true
			break
		}
	}
	if !recursiveForce {
		return false
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		target := strings.TrimSpace(arg)
		switch target {
		case "", "/", ".", "..", "*", ".git", "./.git":
			return true
		}
		if filepath.IsAbs(target) {
			cleanTarget := filepath.Clean(target)
			cleanWorking := filepath.Clean(deliverableDirectory)
			if cleanTarget == cleanWorking || !strings.HasPrefix(cleanTarget, cleanWorking+string(os.PathSeparator)) {
				return true
			}
		}
		if strings.Contains(target, "..") {
			return true
		}
	}
	return false
}

func fuserCommandIsDangerous(args []string) bool {
	for _, arg := range args {
		if strings.Contains(strings.TrimSpace(arg), "k") && strings.HasPrefix(strings.TrimSpace(arg), "-") {
			return true
		}
	}
	return false
}

func chmodCommandIsDangerous(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == "777" {
			return true
		}
	}
	return false
}

func gitCommandIsDangerous(args []string) bool {
	if len(args) < 2 || args[0] != "push" {
		return false
	}
	for _, arg := range args[1:] {
		if arg == "--force" || arg == "-f" || strings.HasPrefix(arg, "--force-") {
			return true
		}
	}
	return false
}

func shellScriptIsDangerous(args []string, allowManagedProcessControl bool) bool {
	for idx, arg := range args {
		if arg != "-c" || idx+1 >= len(args) {
			continue
		}
		script := strings.ToLower(args[idx+1])
		dangerousMarkers := []string{
			"rm -rf /",
			"rm -fr /",
			"rm -rf .git",
			"rm -fr .git",
			"sudo ",
			" shutdown",
			" reboot",
			" poweroff",
			" mkfs",
			"git push --force",
			"git push -f",
			"chmod 777",
		}
		padded := " " + script
		for _, marker := range dangerousMarkers {
			if strings.Contains(padded, marker) {
				return true
			}
		}
		if shellScriptHasForbiddenProcessTermination(padded, allowManagedProcessControl) {
			return true
		}
	}
	return false
}

func shellScriptHasForbiddenProcessTermination(paddedScript string, allowManagedProcessControl bool) bool {
	for _, marker := range []string{
		" fuser -k ",
		" fuser -k",
		" xargs kill",
		" pkill ",
		" killall ",
		" kill -",
	} {
		if strings.Contains(paddedScript, marker) {
			return true
		}
	}
	return !allowManagedProcessControl && strings.Contains(paddedScript, " kill ")
}

func shellScriptStartsDetachedProcess(args []string) bool {
	for idx, arg := range args {
		if arg != "-c" || idx+1 >= len(args) {
			continue
		}
		script := strings.ToLower(strings.TrimSpace(args[idx+1]))
		if script == "" {
			continue
		}
		padded := " " + script + " "
		if strings.Contains(padded, " nohup ") || strings.Contains(padded, " setsid ") {
			return true
		}
		if !shellCommandLineContainsStandaloneAmpersand(script) {
			continue
		}
		if shellScriptWaitsForBackgroundWork(script) {
			continue
		}
		return true
	}
	return false
}

func shellScriptWaitsForBackgroundWork(script string) bool {
	padded := " " + strings.ToLower(strings.TrimSpace(script)) + " "
	trimmed := strings.TrimSpace(padded)
	return strings.Contains(padded, " wait ") ||
		strings.Contains(padded, ";wait ") ||
		strings.Contains(padded, "&&wait ") ||
		strings.HasPrefix(trimmed, "wait ") ||
		strings.HasSuffix(trimmed, " wait") ||
		trimmed == "wait"
}

func shellCommandLineContainsStandaloneAmpersand(commandLine string) bool {
	for idx, value := range commandLine {
		if value != '&' {
			continue
		}
		prevBoundary := idx == 0 || strings.ContainsRune(" \t\r\n;(){}|", rune(commandLine[idx-1]))
		nextIdx := idx + len(string(value))
		nextBoundary := nextIdx >= len(commandLine) || strings.ContainsRune(" \t\r\n;(){}|", rune(commandLine[nextIdx]))
		if prevBoundary && nextBoundary {
			return true
		}
	}
	return false
}

func sanitizeSuccessfulCommandStderr(command string, args []string, exitCode int, stdoutText string, stderrText string) string {
	if exitCode != 0 || strings.TrimSpace(stdoutText) == "" || strings.TrimSpace(stderrText) == "" {
		return stderrText
	}
	if !shellCommandMayEmitCurlProgress(command, args) {
		return stderrText
	}
	cleaned, changed := stripCurlProgressMeter(stderrText)
	if !changed {
		return stderrText
	}
	return cleaned
}

func shellCommandMayEmitCurlProgress(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	if base == "curl" {
		return true
	}
	switch base {
	case "sh", "bash", "zsh":
		for idx, arg := range args {
			if arg != "-c" || idx+1 >= len(args) {
				continue
			}
			if shellScriptContainsCurl(args[idx+1]) {
				return true
			}
		}
	}
	return false
}

func shellScriptContainsCurl(script string) bool {
	fields := strings.Fields(strings.TrimSpace(script))
	for _, field := range fields {
		field = strings.Trim(field, "'\"`();|&")
		if filepath.Base(field) == "curl" {
			return true
		}
	}
	return false
}

func stripCurlProgressMeter(stderrText string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(stderrText, "\r", "\n"), "\n")
	ret := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		if isCurlProgressMeterLine(line) {
			changed = true
			continue
		}
		ret = append(ret, line)
	}
	if !changed {
		return stderrText, false
	}
	return strings.TrimSpace(strings.Join(ret, "\n")), true
}

func isCurlProgressMeterLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "% Total") && strings.Contains(trimmed, "% Received") {
		return true
	}
	if strings.Contains(trimmed, "Dload") && strings.Contains(trimmed, "Upload") && strings.Contains(trimmed, "Speed") {
		return true
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 10 {
		return false
	}
	if !allDigits(fields[0]) {
		return false
	}
	return strings.Contains(trimmed, "--:--:--") || strings.Contains(trimmed, "k") || strings.Contains(trimmed, "M")
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
