package ssh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeAdapter struct {
	timeout time.Duration
}

const MaxCommandOutputBytes = 64 * 1024

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{timeout: 30 * time.Second}
}

func NewRuntimeAdapterWithTimeout(timeout time.Duration) *RuntimeAdapter {
	adapter := NewRuntimeAdapter()
	if timeout > 0 {
		adapter.timeout = timeout
	}
	return adapter
}

func (a *RuntimeAdapter) ExecuteOperation(ctx context.Context, request RuntimeCommandRequest) (RuntimeCommandResult, error) {
	spec, err := sshCommandSpecFromRequest(request, a.defaultTimeout())
	if err != nil {
		return RuntimeCommandResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	startedAt := time.Now().UTC()
	result, err := executeSSHCommand(runCtx, spec)
	duration := time.Since(startedAt)
	ret := RuntimeCommandResult{
		Status:          "completed",
		Host:            spec.Host,
		Port:            int64(spec.Port),
		Username:        spec.Username,
		Command:         spec.Command,
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        int64(result.ExitCode),
		DurationMillis:  duration.Milliseconds(),
		TimeoutMillis:   spec.Timeout.Milliseconds(),
		AuthMethod:      spec.authMethod(),
		OutputTruncated: result.StdoutTruncated || result.StderrTruncated,
		StdoutTruncated: result.StdoutTruncated,
		StderrTruncated: result.StderrTruncated,
		StdoutBytes:     int64(result.StdoutBytes),
		StderrBytes:     int64(result.StderrBytes),
	}
	if runCtx.Err() == context.DeadlineExceeded {
		ret.Status = "timeout"
		ret.FailureCode = "ssh_command_timeout"
		ret.FailureMessage = context.DeadlineExceeded.Error()
		ret.Evidence = sshRuntimeEvidence(ret)
		return ret, context.DeadlineExceeded
	}
	if err != nil {
		ret.Status = "failed"
		ret.FailureCode = "ssh_command_failed"
		ret.FailureMessage = err.Error()
		ret.Evidence = sshRuntimeEvidence(ret)
		return ret, err
	}
	ret.Evidence = sshRuntimeEvidence(ret)
	return ret, nil
}

// ProbeOperation validates all local SSH execution prerequisites without
// opening a network connection or starting a command. It is used exclusively
// by Runtime candidate admission before a dispatch is sealed.
func (a *RuntimeAdapter) ProbeOperation(request RuntimeCommandRequest) error {
	spec, err := sshCommandSpecFromRequest(request, a.defaultTimeout())
	if err != nil {
		return err
	}
	if spec.privateKeyPath == "" {
		return nil
	}
	info, err := os.Stat(spec.privateKeyPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: ssh privateKeyPath is not a regular file", common.ErrInvalidArgument)
	}
	return nil
}

func sshRuntimeEvidence(result RuntimeCommandResult) runtimecontract.RuntimeOperationEvidence {
	exitCode := result.ExitCode
	return runtimecontract.RuntimeOperationEvidence{
		Summary:  strings.TrimSpace(result.Status + " ssh command"),
		ExitCode: &exitCode,
	}
}

func (a *RuntimeAdapter) defaultTimeout() time.Duration {
	if a == nil || a.timeout <= 0 {
		return 30 * time.Second
	}
	return a.timeout
}

// admissibilityRevision is a stable identity for local SSH execution policy;
// it deliberately excludes transient network reachability and credentials.
func (a *RuntimeAdapter) admissibilityRevision() string {
	hash := sha256.Sum256([]byte("ssh-policy|timeout=" + a.defaultTimeout().String() + "|private-key-regular-file"))
	return "ssh-policy-" + hex.EncodeToString(hash[:])[:20]
}
