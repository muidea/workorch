package shell

import (
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

func (a *RuntimeAdapter) ExecuteOperation(ctx context.Context, request RuntimeCommandRequest) (RuntimeCommandResult, error) {
	command, args, err := shellCommandSpec(request.Command, request.Args)
	if err != nil {
		return RuntimeCommandResult{}, err
	}
	deliverableDirectory, err := filepath.Abs(strings.TrimSpace(request.WorkspaceDeliverableDirectory))
	if err != nil || deliverableDirectory == "" {
		return RuntimeCommandResult{}, common.ErrInvalidArgument
	}
	if err := os.MkdirAll(deliverableDirectory, 0o755); err != nil {
		return RuntimeCommandResult{}, err
	}
	managedValidation := shellInputHasManagedValidation(request)
	execCommand, execArgs, commandNormalization := normalizeShellExecutionCommand(command, args)
	if err := validateShellCommandPolicy(execCommand, execArgs, deliverableDirectory, managedValidation); err != nil {
		now := time.Now().UTC()
		output := shellBaseOutput(request, command, args, deliverableDirectory, commandNormalization, execCommand)
		output.Status = "failed"
		output.Stage = "shell_command_policy_rejected"
		output.Reason = err.Error()
		attachManagedValidationResources(&output, request, 0, now, now, "released")
		attachShellValidationFailureDiagnostic(&output, request, execCommand, 0, false, true, err)
		return finalizeShellResult(output, err)
	}
	if err := validateValidationShellEvidencePolicy(request.ValidationEvidence, execCommand, execArgs); err != nil {
		now := time.Now().UTC()
		output := shellBaseOutput(request, command, args, deliverableDirectory, commandNormalization, execCommand)
		output.Status = "failed"
		output.Stage = "shell_validation_contract_rejected"
		output.Reason = err.Error()
		output.FailureReason = "validation_shell_masks_failure"
		attachManagedValidationResources(&output, request, 0, now, now, "released")
		attachShellValidationFailureDiagnostic(&output, request, execCommand, 0, false, true, err)
		return finalizeShellResult(output, err)
	}

	timeout, err := shellCommandTimeout(request.TimeoutMillis, a.timeout)
	if err != nil {
		now := time.Now().UTC()
		output := shellBaseOutput(request, command, args, deliverableDirectory, commandNormalization, execCommand)
		output.Status = "failed"
		output.Stage = "shell_command_timeout_policy_rejected"
		output.Reason = err.Error()
		attachManagedValidationResources(&output, request, 0, now, now, "released")
		attachShellValidationFailureDiagnostic(&output, request, execCommand, 0, false, true, err)
		return finalizeShellResult(output, err)
	}
	if err := validateShellValidationPreflight(deliverableDirectory, execCommand, execArgs, request); err != nil {
		now := time.Now().UTC()
		output := shellBaseOutput(request, command, args, deliverableDirectory, commandNormalization, execCommand)
		output.Status = "failed"
		output.Stage = "shell_validation_preflight_failed"
		output.Reason = err.Error()
		output.FailureReason = "validation_target_missing"
		output.FailureCode = "validation_target_missing"
		output.EvidenceQuality = string(common.EvidenceQualityInconclusive)
		output.EvidenceReason = err.Error()
		attachManagedValidationResources(&output, request, 0, now, now, "released")
		attachShellValidationFailureDiagnostic(&output, request, execCommand, 0, false, true, err)
		return finalizeShellResult(output, err)
	}
	if request.StateChanging || managedValidation {
		if err := validateShellWorkspaceSymlinkScope(deliverableDirectory); err != nil {
			now := time.Now().UTC()
			output := shellBaseOutput(request, command, args, deliverableDirectory, commandNormalization, execCommand)
			output.Status = "failed"
			output.Stage = "shell_workspace_scope_rejected"
			output.Reason = err.Error()
			output.FailureReason = "workspace_symlink_scope_invalid"
			attachManagedValidationResources(&output, request, 0, now, now, "failed")
			attachShellValidationFailureDiagnostic(&output, request, execCommand, 0, false, true, err)
			return finalizeShellResult(output, err)
		}
	}
	beforeChange := workspaceStateChangeFingerprint(ctx, deliverableDirectory, request.StateChanging)
	beforeTargets := shellWorkspaceTargetRevisions(deliverableDirectory, request)
	beforeWorkspace, observationErr := shellWorkspaceFileRevisions(deliverableDirectory, request.StateChanging || managedValidation)
	if observationErr != nil {
		return RuntimeCommandResult{}, observationErr
	}
	executionDirectory := deliverableDirectory
	if managedValidation {
		executionDirectory, err = prepareManagedValidationSandbox(&request, deliverableDirectory)
		if err != nil {
			now := time.Now().UTC()
			output := shellBaseOutput(request, command, args, deliverableDirectory, commandNormalization, execCommand)
			output.Status = "failed"
			output.Stage = "managed_validation_sandbox_unavailable"
			output.Reason = err.Error()
			output.FailureReason = "managed_validation_sandbox_unavailable"
			attachManagedValidationResources(&output, request, 0, now, now, "failed")
			attachShellValidationFailureDiagnostic(&output, request, execCommand, 0, false, true, err)
			return finalizeShellResult(output, err)
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startedAt := time.Now().UTC()
	cmd := exec.CommandContext(runCtx, execCommand, execArgs...)
	cmd.Dir = executionDirectory
	cmd.Env = a.filteredEnv()
	if managedValidation {
		cmd.Env = managedValidationEnvironment(cmd.Env, request.ValidationScratchDirectory)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return terminateShellProcessGroup(cmd.Process.Pid)
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
	processCleanupErr := terminateShellProcessGroup(pid)
	processResourceStatus := "released"
	if runCtx.Err() == context.DeadlineExceeded {
		processResourceStatus = "timeout"
	}
	if processCleanupErr != nil {
		processResourceStatus = "cleanup_failed"
	}
	if err == nil && processCleanupErr != nil {
		err = fmt.Errorf("shell process group cleanup failed: %w", processCleanupErr)
	}
	duration := time.Since(startedAt)
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	stdoutText := stdout.String()
	stderrText := stderr.String()
	output := shellBaseOutput(request, command, args, executionDirectory, commandNormalization, execCommand)
	output.Status = "completed"
	output.Stdout = stdoutText
	output.Stderr = sanitizeSuccessfulCommandStderr(command, args, exitCode, stdoutText, stderrText)
	output.ExitCode = int64(exitCode)
	output.DurationMillis = duration.Milliseconds()
	output.TimeoutMillis = timeout.Milliseconds()
	if stdout.Truncated() || stderr.Truncated() {
		output.OutputTruncated = stdout.Truncated() || stderr.Truncated()
		output.StdoutTruncated = stdout.Truncated()
		output.StderrTruncated = stderr.Truncated()
		output.StdoutBytes = int64(stdout.TotalBytes())
		output.StderrBytes = int64(stderr.TotalBytes())
		output.MaxOutputBytes = MaxCommandOutputBytes
	}
	attachShellValidationObservation(&output, command, args, request.ValidationRecipeID, managedValidation)
	applyShellEvidencePolicies(request.EvidencePolicies, command, args, request.ValidationRecipeID, &output)
	if runCtx.Err() == context.DeadlineExceeded {
		output.Status = "timeout"
		output.Stage = "shell_command_timeout"
		attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), processResourceStatus)
		attachShellValidationFailureDiagnostic(&output, request, execCommand, exitCode, true, false, context.DeadlineExceeded)
		return finalizeShellResult(output, context.DeadlineExceeded)
	}
	if err != nil {
		output.Status = "failed"
		attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), processResourceStatus)
		attachShellCommandFailureDiagnostics(&output, execCommand, err)
		if output.FailureCode == "" && output.FailureReason == "" && output.Stage == "" {
			output.Stage = "shell_command_failed"
			output.FailureReason = "shell_exit_nonzero"
			output.FailureCode = "shell_exit_nonzero"
		}
		attachShellValidationFailureDiagnostic(&output, request, execCommand, exitCode, false, false, err)
		return finalizeShellResult(output, err)
	}
	afterWorkspace, observeErr := shellWorkspaceFileRevisions(deliverableDirectory, request.StateChanging || managedValidation)
	if observeErr != nil {
		err := fmt.Errorf("%w: Shell workspace observation failed: %v", common.ErrInvalidArgument, observeErr)
		output.Status = "failed"
		output.Stage = "shell_workspace_observation_unavailable"
		output.FailureReason = "workspace_observation_unavailable"
		attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), processResourceStatus)
		attachShellValidationFailureDiagnostic(&output, request, execCommand, exitCode, false, false, err)
		return finalizeShellResult(output, err)
	}
	output.WorkspaceObservedTargetChanges = shellWorkspaceTargetChanges(beforeWorkspace, afterWorkspace)
	if managedValidation && shellWorkspaceTargetChangesChanged(output.WorkspaceObservedTargetChanges) {
		err := fmt.Errorf("%w: managed validation changed authoritative workspace state", common.ErrInvalidArgument)
		output.Status = "failed"
		output.Stage = "managed_validation_workspace_effect"
		output.FailureReason = "managed_validation_workspace_effect"
		output.EvidenceQuality = string(common.EvidenceQualityInvalid)
		output.EvidenceReason = "managed validation escaped its owner-isolated workspace"
		attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), processResourceStatus)
		attachShellValidationFailureDiagnostic(&output, request, execCommand, exitCode, false, false, err)
		return finalizeShellResult(output, err)
	}
	if request.StateChanging {
		afterTargets := shellWorkspaceTargetRevisions(deliverableDirectory, request)
		output.WorkspaceTargetChanges = shellWorkspaceTargetChanges(beforeTargets, afterTargets)
		if !shellWorkspaceTargetChangesChanged(output.WorkspaceObservedTargetChanges) {
			err := fmt.Errorf("%w: state-changing shell command produced no observable workspace delta for its declared targets", common.ErrInvalidArgument)
			output.Status = "failed"
			output.Stage = "shell_state_changing_no_progress"
			output.FailureReason = "state_changing_no_workspace_delta"
			output.ZeroWork = true
			output.EvidenceQuality = string(common.EvidenceQualityInconclusive)
			output.EvidenceReason = "state-changing shell command completed without changing an explicitly declared workspace target"
			output.RuntimeDiagnostics = runtimemodel.RuntimeDiagnostics{
				Kind:          "state_changing_no_progress",
				Operation:     "shell/command",
				TargetRef:     strings.TrimSpace(request.TargetRef),
				FailureReason: "state_changing_no_workspace_delta",
			}
			attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), processResourceStatus)
			attachShellValidationFailureDiagnostic(&output, request, execCommand, exitCode, false, false, err)
			return finalizeShellResult(output, err)
		}
	}
	if beforeChange.Available {
		afterChange := workspaceStateChangeFingerprint(ctx, deliverableDirectory, true)
		if afterChange.Available {
			output.WorkspaceStateChangeFingerprint = runtimemodel.RuntimeWorkspaceStateChangeFingerprint{
				Before: beforeChange.Value,
				After:  afterChange.Value,
			}
			if beforeChange.Value == afterChange.Value {
				err := fmt.Errorf("%w: state-changing shell command produced no observable workspace delta", common.ErrInvalidArgument)
				output.Status = "failed"
				output.Stage = "shell_state_changing_no_progress"
				output.FailureReason = "state_changing_no_workspace_delta"
				output.ZeroWork = true
				output.EvidenceQuality = string(common.EvidenceQualityInconclusive)
				output.EvidenceReason = "state-changing shell command completed without changing tracked workspace state, untracked files, or HEAD"
				output.RuntimeDiagnostics = runtimemodel.RuntimeDiagnostics{
					Kind:          "state_changing_no_progress",
					Operation:     "shell/command",
					TargetRef:     strings.TrimSpace(request.TargetRef),
					FailureReason: "state_changing_no_workspace_delta",
				}
				attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), processResourceStatus)
				attachShellValidationFailureDiagnostic(&output, request, execCommand, exitCode, false, false, err)
				return finalizeShellResult(output, err)
			}
		}
	}
	attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), processResourceStatus)
	return finalizeShellResult(output, nil)
}

func terminateShellProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
