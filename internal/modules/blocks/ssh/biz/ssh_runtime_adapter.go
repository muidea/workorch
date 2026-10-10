package ssh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeAdapter struct {
	timeout     time.Duration
	targets     map[string]sshTargetSnapshot
	sessions    map[string]sshSessionSnapshot
	credentials map[string]sshCredentialSnapshot
	closeOnce   sync.Once
	closeDone   chan struct{}
	revision    string
}

const MaxCommandOutputBytes = 64 * 1024

func NewRuntimeAdapter() *RuntimeAdapter {
	adapter := &RuntimeAdapter{timeout: 30 * time.Second, targets: map[string]sshTargetSnapshot{}, sessions: map[string]sshSessionSnapshot{}, credentials: map[string]sshCredentialSnapshot{}}
	adapter.revision = stableSSHOwnerPolicyRevisionForEmpty(adapter.timeout)
	return adapter
}

func NewRuntimeAdapterWithTimeout(timeout time.Duration) *RuntimeAdapter {
	adapter := NewRuntimeAdapter()
	if timeout > 0 {
		adapter.timeout = timeout
		adapter.revision = stableSSHOwnerPolicyRevisionForEmpty(timeout)
	}
	return adapter
}

func (a *RuntimeAdapter) ExecuteOperation(ctx context.Context, request RuntimeCommandRequest) (RuntimeCommandResult, error) {
	spec, err := a.sshCommandSpecFromRequest(request)
	if err != nil {
		return RuntimeCommandResult{Status: "failed", TargetRef: strings.TrimSpace(request.TargetRef), FailureCode: "ssh_owner_binding_invalid", FailureMessage: "SSH owner reference binding is invalid"}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	startedAt := time.Now().UTC()
	beforeSpec := spec
	beforeSpec.Command = spec.ObservationCommand
	before, beforeErr := executeSSHCommand(runCtx, beforeSpec)
	if beforeErr != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return sshFailedCommandResult(spec, startedAt, before, "ssh_command_timeout", "SSH execution exceeded the sealed timeout"), context.DeadlineExceeded
		}
		return sshFailedCommandResult(spec, startedAt, before, "ssh_observation_before_failed", "SSH owner could not observe the remote resource before execution"), beforeErr
	}
	beforeHash := sshObservationHash(before)
	commandResult, commandErr := executeSSHCommand(runCtx, spec)
	afterSpec := spec
	afterSpec.Command = spec.ObservationCommand
	after, afterErr := executeSSHCommand(runCtx, afterSpec)
	if afterErr != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			ret := sshFailedCommandResult(spec, startedAt, commandResult, "ssh_command_timeout", "SSH execution exceeded the sealed timeout")
			ret.BeforeHash = beforeHash
			return ret, context.DeadlineExceeded
		}
		ret := sshFailedCommandResult(spec, startedAt, commandResult, "ssh_observation_after_failed", "SSH owner could not observe the remote resource after execution")
		ret.BeforeHash = beforeHash
		ret.OutputHash = sshCommandOutputHash(commandResult)
		return ret, afterErr
	}
	afterHash := sshObservationHash(after)
	changed := beforeHash != afterHash
	duration := time.Since(startedAt).Milliseconds()
	ret := RuntimeCommandResult{
		Status: "completed", TargetRef: spec.TargetRef, ObservationRef: spec.ObservationRef,
		ExitCode: int64(commandResult.ExitCode), DurationMillis: duration,
		TimeoutMillis:   spec.Timeout.Milliseconds(),
		OutputTruncated: commandResult.StdoutTruncated || commandResult.StderrTruncated,
		StdoutTruncated: commandResult.StdoutTruncated, StderrTruncated: commandResult.StderrTruncated,
		StdoutBytes: int64(commandResult.StdoutBytes), StderrBytes: int64(commandResult.StderrBytes),
		OutputHash: sshCommandOutputHash(commandResult), BeforeHash: beforeHash, AfterHash: afterHash, Changed: changed,
	}
	if runCtx.Err() == context.DeadlineExceeded {
		ret.Status = "timeout"
		ret.FailureCode = "ssh_command_timeout"
		ret.FailureMessage = "SSH command exceeded the sealed timeout"
		ret.ObservedScope = runtimecontract.RuntimeEffectUnknown
		ret.Evidence = sshRuntimeEvidence(ret)
		return ret, context.DeadlineExceeded
	}
	if commandErr != nil {
		ret.Status = "failed"
		ret.FailureCode = "ssh_command_failed"
		ret.FailureMessage = "SSH command returned a non-zero or transport failure"
		ret.ObservedScope = runtimecontract.RuntimeEffectNone
		if changed {
			ret.ObservedScope = runtimecontract.RuntimeEffectRemoteDurable
		}
		ret.Evidence = sshRuntimeEvidence(ret)
		return ret, commandErr
	}
	if changed {
		ret.ObservedScope = runtimecontract.RuntimeEffectRemoteDurable
	} else {
		ret.ObservedScope = runtimecontract.RuntimeEffectNone
	}
	ret.Evidence = sshRuntimeEvidence(ret)
	return ret, nil
}

// ProbeOperation validates all local SSH execution prerequisites without
// opening a network connection or starting a command. It is used exclusively
// by Runtime candidate admission before a dispatch is sealed.
func (a *RuntimeAdapter) ProbeOperation(request RuntimeCommandRequest) error {
	_, err := a.sshCommandSpecFromRequest(request)
	return err
}

func sshRuntimeEvidence(result RuntimeCommandResult) runtimecontract.RuntimeOperationEvidence {
	exitCode := result.ExitCode
	refs := []string{}
	if result.OutputHash != "" {
		refs = append(refs, "workorch://ssh/evidence/"+result.OutputHash)
	}
	if result.AfterHash != "" && result.TargetRef != "" && result.ObservationRef != "" {
		refs = append(refs, "workorch://ssh/observations/"+result.TargetRef+"/"+result.ObservationRef+"/"+result.AfterHash)
	}
	summary := fmt.Sprintf("%s SSH command; durationMillis=%d stdoutBytes=%d stderrBytes=%d truncated=%t", strings.TrimSpace(result.Status), result.DurationMillis, result.StdoutBytes, result.StderrBytes, result.OutputTruncated)
	return runtimecontract.RuntimeOperationEvidence{
		Summary: strings.TrimSpace(summary), ContentHash: result.OutputHash, ExitCode: &exitCode, EvidenceRefs: refs,
		ZeroWork: result.Status == "completed" && !result.Changed,
	}
}

func (a *RuntimeAdapter) defaultTimeout() time.Duration {
	if a == nil || a.timeout <= 0 {
		return 30 * time.Second
	}
	return a.timeout
}

// admissibilityRevision is a stable identity for the frozen owner policy. It
// contains only hashes for host identity and secret material, never plaintext.
func (a *RuntimeAdapter) admissibilityRevision() string {
	if a != nil && strings.TrimSpace(a.revision) != "" {
		return a.revision
	}
	return stableSSHOwnerPolicyRevisionForEmpty(a.defaultTimeout())
}

// CloseChecked 独立等待同一次真实清理，超时不清空 owner 引用或重复关闭。
func (a *RuntimeAdapter) CloseChecked(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	a.closeOnce.Do(func() {
		a.closeDone = make(chan struct{})
		go func() {
			for ref, credential := range a.credentials {
				credential.password = ""
				credential.privateKey = ""
				credential.secretHash = ""
				a.credentials[ref] = credential
			}
			a.targets = nil
			a.sessions = nil
			a.credentials = nil
			close(a.closeDone)
		}()
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.closeDone:
		return ctx.Err()
	}
}
func (a *RuntimeAdapter) Close() { _ = a.CloseChecked(context.Background()) }

func stableSSHOwnerPolicyRevisionForEmpty(timeout time.Duration) string {
	hash := sha256.Sum256([]byte("ssh-owner-policy|timeout=" + timeout.String() + "|strict-host-key|owner-observation|approval"))
	return "ssh-policy-" + hex.EncodeToString(hash[:])[:20]
}

func sshObservationHash(result sshCommandResult) string {
	payload := fmt.Sprintf("stdout:%d:%s|stderr:%d:%s|exit:%d", result.StdoutBytes, result.StdoutHash, result.StderrBytes, result.StderrHash, result.ExitCode)
	return hashSSHBytes([]byte(payload))
}

func sshCommandOutputHash(result sshCommandResult) string {
	payload := fmt.Sprintf("stdout:%d:%s|stderr:%d:%s", result.StdoutBytes, result.StdoutHash, result.StderrBytes, result.StderrHash)
	return hashSSHBytes([]byte(payload))
}

func sshFailedCommandResult(spec sshCommandSpec, startedAt time.Time, output sshCommandResult, code, message string) RuntimeCommandResult {
	ret := RuntimeCommandResult{
		Status: "failed", TargetRef: spec.TargetRef, ObservationRef: spec.ObservationRef,
		ExitCode: int64(output.ExitCode), DurationMillis: time.Since(startedAt).Milliseconds(), TimeoutMillis: spec.Timeout.Milliseconds(),
		OutputTruncated: output.StdoutTruncated || output.StderrTruncated, StdoutTruncated: output.StdoutTruncated, StderrTruncated: output.StderrTruncated,
		StdoutBytes: int64(output.StdoutBytes), StderrBytes: int64(output.StderrBytes), OutputHash: sshCommandOutputHash(output),
		ObservedScope: runtimecontract.RuntimeEffectUnknown, FailureCode: code, FailureMessage: message,
	}
	ret.Evidence = sshRuntimeEvidence(ret)
	return ret
}
