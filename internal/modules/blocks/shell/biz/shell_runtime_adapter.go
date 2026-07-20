package shell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// AcquireRuntimeOperationAdmissibility reports the Shell owner's bounded
// command-parameter envelope. It only inspects the workspace directory; no
// command is started and no managed-validation resource is allocated.
func (a *RuntimeAdapter) AcquireRuntimeOperationAdmissibility(workspaceDirectory, prerequisiteID string, operationKinds []string) (runtimemodel.RuntimeOperationAdmissibility, error) {
	directory, err := filepath.Abs(strings.TrimSpace(workspaceDirectory))
	if err != nil || directory == "" {
		return runtimemodel.RuntimeOperationAdmissibility{}, common.ErrInvalidArgument
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = common.ErrInvalidArgument
		}
		return runtimemodel.RuntimeOperationAdmissibility{}, err
	}
	inventory := runtimemodel.RuntimeOperationAdmissibility{
		PrerequisiteID: prerequisiteID,
		OwnerModuleID:  runtimemodel.RuntimeModuleShell,
		OperationKinds: append([]string(nil), operationKinds...),
		ConstraintRefs: []string{"workspace-working-directory", "executable-on-path", "bounded-timeout", "validation-target-must-exist"},
		// Directory mtime changes whenever a workspace child is created. It is
		// neither Shell policy nor a validation-input revision, and using it here
		// turns a preceding File effect into a false candidate failure. Shell's
		// inventory is instead bound to its own stable execution policy.
		ObservedRevision: a.runtimeAdmissibilityPolicyRevision(),
	}
	inventory.SnapshotID = runtimemodel.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		return runtimemodel.RuntimeOperationAdmissibility{}, err
	}
	return inventory, nil
}

// ProbeRuntimeOperationFeasibility validates a concrete shell operation without
// starting a process. It deliberately shares the execution policy helpers so
// candidate admission and owner execution cannot disagree on basic viability.
func (a *RuntimeAdapter) ProbeRuntimeOperationFeasibility(workspaceDirectory string, inventory runtimemodel.RuntimeOperationAdmissibility, operation runtimemodel.RuntimeOperation) *runtimemodel.FailureFact {
	failure, _ := a.ProbeRuntimeOperationFeasibilityWithObservation(workspaceDirectory, inventory, operation, nil)
	return failure
}

func (a *RuntimeAdapter) ProbeRuntimeOperationFeasibilityWithObservation(workspaceDirectory string, inventory runtimemodel.RuntimeOperationAdmissibility, operation runtimemodel.RuntimeOperation, observation *runtimemodel.RuntimePrerequisiteObservation) (*runtimemodel.FailureFact, *runtimemodel.RuntimePrerequisiteStale) {
	operation = operation.Normalized()
	inventory = inventory.Normalized()
	request, err := shellProbeRequestFromOperation(workspaceDirectory, operation)
	if err != nil {
		return shellOperationInfeasible(operation, "shell_operation_invalid", err), nil
	}
	command, args, err := shellCommandSpec(request.Command, request.Args)
	if err != nil {
		return shellOperationInfeasible(operation, "shell_command_invalid", err), nil
	}
	directory, err := filepath.Abs(strings.TrimSpace(request.WorkspaceDeliverableDirectory))
	if err != nil || directory == "" {
		return shellOperationInfeasible(operation, "shell_working_directory_invalid", common.ErrInvalidArgument), nil
	}
	info, statErr := os.Stat(directory)
	if statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = common.ErrInvalidArgument
		}
		return shellOperationInfeasible(operation, "shell_working_directory_unavailable", statErr), nil
	}
	if inventory.ObservedRevision != a.runtimeAdmissibilityPolicyRevision() {
		if observation != nil {
			value := observation.Normalized()
			for _, dependency := range value.Dependencies {
				if dependency.Domain == runtimemodel.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == runtimemodel.RuntimeModuleShell && dependency.SubjectRef == "shell_runtime_policy" && dependency.Revision == inventory.ObservedRevision {
					return nil, &runtimemodel.RuntimePrerequisiteStale{PrerequisiteID: value.PrerequisiteID, ObservationID: value.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: a.runtimeAdmissibilityPolicyRevision(), Reason: "runtime shell operation policy changed"}
				}
			}
		}
		return shellOperationInfeasible(operation, "operation_admissibility_stale", fmt.Errorf("runtime shell operation admissibility policy is stale")), nil
	}
	if observation != nil {
		for _, dependency := range observation.Normalized().Dependencies {
			// The Shell owner observes this target, but File is the owner that
			// reports durable workspace_target revisions. The dependency must use
			// the resource owner so a File receipt intersects it exactly.
			if dependency.Domain != runtimemodel.RuntimeResourceWorkspaceTarget || dependency.OwnerModuleID != runtimemodel.RuntimeModuleFile {
				continue
			}
			if observedRevision, ok := shellWorkspaceTargetRevision(directory, dependency.SubjectRef); !ok || observedRevision != dependency.Revision {
				if !ok {
					observedRevision = "missing"
				}
				return nil, &runtimemodel.RuntimePrerequisiteStale{PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: observedRevision, Reason: "runtime shell validation target changed after admissibility acquisition"}
			}
		}
	}
	execCommand, execArgs, _ := normalizeShellExecutionCommand(command, args)
	if _, lookErr := exec.LookPath(execCommand); lookErr != nil {
		return shellOperationInfeasible(operation, "shell_executable_unavailable", lookErr), nil
	}
	if err := validateShellCommandPolicy(execCommand, execArgs, directory, shellInputHasManagedValidation(request)); err != nil {
		return shellOperationInfeasible(operation, "shell_command_policy_rejected", err), nil
	}
	if err := validateValidationShellEvidencePolicy(request.ValidationEvidence, execCommand, execArgs); err != nil {
		return shellOperationInfeasible(operation, "shell_validation_contract_rejected", err), nil
	}
	if _, err := shellCommandTimeout(request.TimeoutMillis, a.timeout); err != nil {
		return shellOperationInfeasible(operation, "shell_command_timeout_policy_rejected", err), nil
	}
	if err := validateShellValidationPreflight(directory, execCommand, execArgs, request); err != nil {
		return shellOperationInfeasible(operation, "validation_target_missing", err), nil
	}
	return nil, nil
}

func (a *RuntimeAdapter) runtimeAdmissibilityPolicyRevision() string {
	keys := make([]string, 0, len(a.envAllowlist))
	for key := range a.envAllowlist {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "timeout=%s;", a.timeout)
	for _, key := range keys {
		_, _ = fmt.Fprintf(h, "env=%s;", key)
	}
	return "shell-policy-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func (a *RuntimeAdapter) runtimeAdmissibilityObservation(workspaceDirectory, prerequisiteID string, inventory runtimemodel.RuntimeOperationAdmissibility, targetRefs []string) runtimemodel.RuntimePrerequisiteObservation {
	dependencies := []runtimemodel.RuntimeFreshnessDependency{{
		Domain: runtimemodel.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimemodel.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", Revision: inventory.ObservedRevision,
	}}
	for _, targetRef := range targetRefs {
		revision, ok := shellWorkspaceTargetRevision(workspaceDirectory, targetRef)
		if !ok {
			// Absence is itself an observed target revision. Keeping it allows a
			// File create receipt to invalidate this Shell observation before a
			// later validation treats the new target as if it had been acquired.
			revision = "missing"
		}
		dependencies = append(dependencies, runtimemodel.RuntimeFreshnessDependency{
			Domain: runtimemodel.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimemodel.RuntimeModuleFile, SubjectRef: strings.TrimSpace(targetRef), Revision: revision,
		})
	}
	observation := runtimemodel.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimemodel.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimemodel.RuntimeModuleShell,
		ContentHash: inventory.SnapshotID, Dependencies: dependencies,
	}
	observation.ObservationID = runtimemodel.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func shellWorkspaceTargetRevision(workspaceDirectory, targetRef string) (string, bool) {
	workspaceDirectory = strings.TrimSpace(workspaceDirectory)
	targetRef = strings.TrimSpace(targetRef)
	if workspaceDirectory == "" || targetRef == "" || filepath.IsAbs(targetRef) {
		return "", false
	}
	root, err := filepath.Abs(workspaceDirectory)
	if err != nil {
		return "", false
	}
	target := filepath.Join(root, filepath.Clean(targetRef))
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", false
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", false
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), true
}

func shellProbeRequestFromOperation(workspaceDirectory string, operation runtimemodel.RuntimeOperation) (RuntimeCommandRequest, error) {
	if operation.Kind != runtimemodel.StepKindShellCommand && operation.Kind != runtimemodel.RuntimeBlockKindManagedValidationSession {
		return RuntimeCommandRequest{}, common.ErrInvalidArgument
	}
	request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspaceDirectory}
	if operation.Kind == runtimemodel.RuntimeBlockKindManagedValidationSession {
		request.ManagedValidation = runtimemodel.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimemodel.RuntimeBlockKindManagedValidationSession, OriginalNodeKind: runtimemodel.RuntimeBlockKindManagedValidationSession}
	}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "command":
			if parameter.String != nil {
				request.Command = *parameter.String
			}
		case "args":
			request.Args = append([]string(nil), parameter.StringList...)
		case "timeoutMillis":
			if parameter.Integer != nil {
				request.TimeoutMillis = *parameter.Integer
			}
		case "stateChanging":
			if parameter.Boolean != nil {
				request.StateChanging = *parameter.Boolean
			}
		case "targetRef":
			if parameter.String != nil {
				request.TargetRef = *parameter.String
			}
		case "targetRefs":
			request.TargetRefs = append([]string(nil), parameter.StringList...)
		}
	}
	return request, nil
}

func shellOperationInfeasible(operation runtimemodel.RuntimeOperation, code string, err error) *runtimemodel.FailureFact {
	message := "shell operation is not currently feasible"
	if err != nil {
		message = err.Error()
	}
	return &runtimemodel.FailureFact{
		Class: runtimemodel.FailureClassCandidateContractFailure,
		Code:  code, Signature: "operation-admissibility/" + operation.Kind + "/" + code,
		Message: message, TargetRefs: append([]string(nil), operation.TargetRefs...),
	}
}

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
	execCommand, execArgs, commandNormalization := normalizeShellExecutionCommand(command, args)
	if err := validateShellCommandPolicy(execCommand, execArgs, deliverableDirectory, shellInputHasManagedValidation(request)); err != nil {
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
	beforeChange := workspaceStateChangeFingerprint(ctx, deliverableDirectory, request.StateChanging)
	beforeTargets := shellWorkspaceTargetRevisions(deliverableDirectory, request)
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
	output := shellBaseOutput(request, command, args, deliverableDirectory, commandNormalization, execCommand)
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
	applyShellEvidencePolicies(request.EvidencePolicies, command, args, &output)
	if runCtx.Err() == context.DeadlineExceeded {
		output.Status = "timeout"
		output.Stage = "shell_command_timeout"
		attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), "timeout")
		attachShellValidationFailureDiagnostic(&output, request, execCommand, exitCode, true, false, context.DeadlineExceeded)
		return finalizeShellResult(output, context.DeadlineExceeded)
	}
	if err != nil {
		output.Status = "failed"
		attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), "failed")
		attachShellCommandFailureDiagnostics(&output, execCommand, err)
		if output.FailureCode == "" && output.FailureReason == "" && output.Stage == "" {
			output.Stage = "shell_command_failed"
			output.FailureReason = "shell_exit_nonzero"
			output.FailureCode = "shell_exit_nonzero"
		}
		attachShellValidationFailureDiagnostic(&output, request, execCommand, exitCode, false, false, err)
		return finalizeShellResult(output, err)
	}
	if request.StateChanging {
		afterTargets := shellWorkspaceTargetRevisions(deliverableDirectory, request)
		output.WorkspaceTargetChanges = shellWorkspaceTargetChanges(beforeTargets, afterTargets)
		if !shellWorkspaceTargetChangesChanged(output.WorkspaceTargetChanges) {
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
			attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), "failed")
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
				attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), "failed")
				attachShellValidationFailureDiagnostic(&output, request, execCommand, exitCode, false, false, err)
				return finalizeShellResult(output, err)
			}
		}
	}
	attachManagedValidationResources(&output, request, pid, startedAt, time.Now().UTC(), "released")
	return finalizeShellResult(output, nil)
}

func shellWorkspaceTargetRevisions(deliverableDirectory string, request RuntimeCommandRequest) map[string]string {
	if !request.StateChanging {
		return nil
	}
	root, err := filepath.Abs(strings.TrimSpace(deliverableDirectory))
	if err != nil || root == "" {
		return nil
	}
	refs := append([]string{request.TargetRef}, request.TargetRefs...)
	ret := make(map[string]string, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || filepath.IsAbs(ref) {
			continue
		}
		target := filepath.Join(root, filepath.Clean(ref))
		if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
			continue
		}
		data, readErr := os.ReadFile(target)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				ret[ref] = "missing"
			}
			continue
		}
		hash := sha256.Sum256(data)
		ret[ref] = hex.EncodeToString(hash[:])
	}
	return ret
}

func shellWorkspaceTargetChanges(before map[string]string, after map[string]string) []RuntimeWorkspaceTargetChange {
	seen := make(map[string]struct{}, len(before)+len(after))
	for ref := range before {
		seen[ref] = struct{}{}
	}
	for ref := range after {
		seen[ref] = struct{}{}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	ret := make([]RuntimeWorkspaceTargetChange, 0, len(refs))
	for _, ref := range refs {
		beforeRevision, afterRevision := before[ref], after[ref]
		if beforeRevision == "" {
			beforeRevision = "missing"
		}
		if afterRevision == "" {
			afterRevision = "missing"
		}
		ret = append(ret, RuntimeWorkspaceTargetChange{TargetRef: ref, BeforeRevision: beforeRevision, AfterRevision: afterRevision})
	}
	return ret
}

func shellWorkspaceTargetChangesChanged(values []RuntimeWorkspaceTargetChange) bool {
	for _, value := range values {
		if strings.TrimSpace(value.TargetRef) != "" && value.BeforeRevision != value.AfterRevision {
			return true
		}
	}
	return false
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

func shellBaseOutput(request RuntimeCommandRequest, command string, args []string, workingDir string, commandNormalization string, execCommand string) RuntimeCommandResult {
	output := RuntimeCommandResult{
		Command:            command,
		Args:               append([]string(nil), args...),
		WorkingDirectory:   workingDir,
		TargetRefs:         normalizeShellStrings(request.TargetRefs),
		ValidationEvidence: cloneRuntimeValidationEvidence(request.ValidationEvidence),
		ManagedValidation:  request.ManagedValidation,
	}
	if commandNormalization != "" {
		output.ActualCommand = filepath.Base(execCommand)
		output.CommandNormalization = commandNormalization
	}
	return output
}

func attachManagedValidationResources(output *RuntimeCommandResult, request RuntimeCommandRequest, pid int, startedAt time.Time, finishedAt time.Time, processStatus string) {
	if output == nil || !shellInputHasValidationResourceLifecycle(request) {
		return
	}
	managedInput := shellInputHasManagedValidation(request)
	resources := make([]runtimemodel.RuntimeValidationResource, 0, 2)
	if pid > 0 {
		resources = append(resources, runtimemodel.RuntimeValidationResource{
			ID:          shellManagedValidationResourceID(request, "process"),
			WorkspaceID: string(request.WorkspaceID), RunID: string(request.RunID), StepID: string(request.StepID),
			Kind: "process", PID: pid, Status: processStatus,
			CleanupPolicy: "process_group_on_timeout_or_completion",
			CreatedAt:     startedAt.Format(time.RFC3339Nano), ReleasedAt: finishedAt.Format(time.RFC3339Nano),
		})
	}
	if scratch := strings.TrimSpace(request.ValidationScratchDirectory); scratch != "" {
		autoCleanup := managedValidationScratchAutoCleanupEnabled(request)
		cleanupPolicy := "remove_step_scoped_validation_scratch_on_completion"
		status := "released"
		if !autoCleanup {
			cleanupPolicy = "retain_validation_scratch_until_run_terminal"
			status = "retained"
		}
		record := runtimemodel.RuntimeValidationResource{
			ID:          shellManagedValidationResourceID(request, "scratch_dir"),
			WorkspaceID: string(request.WorkspaceID), RunID: string(request.RunID), StepID: string(request.StepID),
			Kind: "scratch_dir", Path: scratch, Status: status, CleanupPolicy: cleanupPolicy,
			CreatedAt: startedAt.Format(time.RFC3339Nano), ReleasedAt: finishedAt.Format(time.RFC3339Nano),
		}
		if cleanupErr := cleanupManagedValidationScratch(request); cleanupErr != nil {
			record.Status = "cleanup_failed"
			record.CleanupError = cleanupErr.Error()
		}
		resources = append(resources, record)
	}
	if len(resources) == 0 {
		return
	}
	output.ValidationResources = resources
	released := managedValidationResourcesReleased(resources)
	if managedInput {
		output.ManagedValidation.ResourceCount = len(resources)
		output.ManagedValidation.ResourcesReleased = released
	}
	output.RuntimeDiagnostics.ValidationResourceLifecycle = runtimemodel.RuntimeValidationResourceLifecycle{
		ResourceCount: len(resources), Released: released,
	}
}

func shellInputHasValidationResourceLifecycle(request RuntimeCommandRequest) bool {
	return shellInputHasManagedValidation(request) || strings.TrimSpace(request.ValidationScratchDirectory) != ""
}

func shellInputHasManagedValidation(request RuntimeCommandRequest) bool {
	managed := request.ManagedValidation
	return managed.RuntimeManaged ||
		strings.TrimSpace(managed.Kind) == runtimemodel.RuntimeBlockKindManagedValidationSession ||
		strings.TrimSpace(managed.OriginalNodeKind) == runtimemodel.RuntimeBlockKindManagedValidationSession
}

func shellManagedValidationResourceID(request RuntimeCommandRequest, kind string) string {
	return strings.Join([]string{string(request.WorkspaceID), string(request.RunID), string(request.StepID), strings.TrimSpace(kind)}, ":")
}

func managedValidationResourcesReleased(resources []runtimemodel.RuntimeValidationResource) bool {
	for _, resource := range resources {
		switch strings.TrimSpace(resource.Status) {
		case "released", "timeout", "failed":
		default:
			return false
		}
	}
	return true
}

func cleanupManagedValidationScratch(request RuntimeCommandRequest) error {
	if !managedValidationScratchAutoCleanupEnabled(request) {
		return nil
	}
	scratch := strings.TrimSpace(request.ValidationScratchDirectory)
	if scratch == "" {
		return nil
	}
	scratchAbs, err := filepath.Abs(scratch)
	if err != nil {
		return err
	}
	deliverableAbs, err := filepath.Abs(strings.TrimSpace(request.WorkspaceDeliverableDirectory))
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

func managedValidationScratchAutoCleanupEnabled(request RuntimeCommandRequest) bool {
	if request.ManagedValidation.AutoCleanupSet {
		return request.ManagedValidation.AutoCleanup
	}
	if request.ValidationScratchAutoCleanupSet {
		return request.ValidationScratchAutoCleanup
	}
	return strings.TrimSpace(request.ValidationScratchDirectory) != ""
}

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

func shellCommandTimeout(timeoutMillis int64, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = DefaultCommandTimeout
	}
	if timeoutMillis == 0 {
		return defaultTimeout, nil
	}
	if timeoutMillis < 1000 {
		return 0, fmt.Errorf("%w: timeoutMillis must be at least 1000", common.ErrInvalidArgument)
	}
	if timeoutMillis > int64(MaxCommandTimeout/time.Millisecond) {
		return 0, fmt.Errorf("%w: timeoutMillis must not exceed %d", common.ErrInvalidArgument, MaxCommandTimeout/time.Millisecond)
	}
	return time.Duration(timeoutMillis) * time.Millisecond, nil
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

// validateShellValidationPreflight performs domain-neutral executability checks for validation-like shell commands.
// It does not encode product-domain test frameworks beyond generic package/tool presence signals.
func validateShellValidationPreflight(deliverableDirectory string, command string, args []string, request RuntimeCommandRequest) error {
	if !shellRequestLooksLikeValidation(request, command, args) {
		return nil
	}
	base := filepath.Base(strings.TrimSpace(command))
	switch base {
	case "go":
		return validateGoTestValidationPreflight(deliverableDirectory, args)
	default:
		return nil
	}
}

func shellRequestLooksLikeValidation(request RuntimeCommandRequest, command string, args []string) bool {
	if !runtimeValidationEvidenceEmpty(request.ValidationEvidence) {
		return true
	}
	// Heuristic for validation-shaped go test invocations used as verification evidence.
	if filepath.Base(strings.TrimSpace(command)) == "go" && len(args) > 0 && strings.TrimSpace(args[0]) == "test" {
		return true
	}
	return false
}

func validateGoTestValidationPreflight(deliverableDirectory string, args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) != "test" {
		return nil
	}
	if err := validateGoTestPackageTargets(deliverableDirectory, args); err != nil {
		return err
	}
	runPattern := goTestRunPattern(args)
	hasTestFiles, err := workspaceContainsGoTestFiles(deliverableDirectory)
	if err != nil {
		return err
	}
	if !hasTestFiles {
		if runPattern != "" {
			return fmt.Errorf("%w: validation preflight failed: no *_test.go files found for go test -run %q; create a matching test first or choose an executable validation command", common.ErrInvalidArgument, runPattern)
		}
		return fmt.Errorf("%w: validation preflight failed: no *_test.go files found for go test; create tests first or choose an executable validation command", common.ErrInvalidArgument)
	}
	return nil
}

func validateGoTestPackageTargets(deliverableDirectory string, args []string) error {
	for idx := 1; idx < len(args); idx++ {
		arg := strings.TrimSpace(args[idx])
		if arg == "" {
			continue
		}
		if arg == "-run" || arg == "--run" {
			idx++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Go package patterns not rooted in the workspace are resolved by the Go
		// toolchain. Workspace-relative targets are deterministic and can be
		// rejected before process start when they do not exist.
		if arg != "." && !strings.HasPrefix(arg, "./") {
			continue
		}
		if arg == "./..." {
			continue
		}
		rel := strings.TrimPrefix(arg, "./")
		path := filepath.Join(deliverableDirectory, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			if err == nil {
				err = common.ErrNotFound
			}
			return fmt.Errorf("%w: validation preflight failed: go test package target %q does not exist", err, arg)
		}
	}
	return nil
}

func goTestRunPattern(args []string) string {
	for idx := 0; idx < len(args); idx++ {
		arg := strings.TrimSpace(args[idx])
		switch {
		case arg == "-run" || arg == "--run":
			if idx+1 < len(args) {
				return strings.TrimSpace(args[idx+1])
			}
		case strings.HasPrefix(arg, "-run="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "-run="))
		case strings.HasPrefix(arg, "--run="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "--run="))
		}
	}
	return ""
}

func workspaceContainsGoTestFiles(root string) (bool, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return false, common.ErrInvalidArgument
	}
	found := false
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Permission or transient path errors should not hard-fail preflight broadly.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "vendor" || name == "node_modules" || name == ".workorch-data" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, "_test.go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return false, err
	}
	return found, nil
}

func shellCommandSpec(command string, args []string) (string, []string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, common.ErrInvalidArgument
	}
	return command, append([]string(nil), args...), nil
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

func validateValidationShellEvidencePolicy(evidence runtimemodel.RuntimeValidationEvidence, command string, args []string) error {
	if runtimeValidationEvidenceEmpty(evidence) {
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

func runtimeValidationEvidenceEmpty(value runtimemodel.RuntimeValidationEvidence) bool {
	return strings.TrimSpace(value.Mode) == "" &&
		strings.TrimSpace(value.ObservationType) == "" &&
		strings.TrimSpace(value.Observation) == "" &&
		strings.TrimSpace(value.DiagnosticClass) == "" &&
		strings.TrimSpace(value.FailureClass) == "" &&
		strings.TrimSpace(value.ExpectedFailureClass) == "" &&
		strings.TrimSpace(value.RecommendedEvidenceTargetType) == "" &&
		len(value.TargetRefs) == 0 && len(value.CoverageTargets) == 0 && len(value.ObservedClaims) == 0
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
	output.Evidence = runtimemodel.RuntimeOperationEvidence{Summary: summary, ExitCode: &exitCode}
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
