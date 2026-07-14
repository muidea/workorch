package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

type sshCommandSpec struct {
	Host           string
	Port           int
	Username       string
	Password       string
	PrivateKey     string
	privateKeyPath string
	Command        string
	Timeout        time.Duration
}

func (s sshCommandSpec) authMethod() string {
	if strings.TrimSpace(s.PrivateKey) != "" || strings.TrimSpace(s.privateKeyPath) != "" {
		return "private_key"
	}
	if strings.TrimSpace(s.Password) != "" {
		return "password"
	}
	return ""
}

func sshCommandSpecFromRequest(request RuntimeCommandRequest, defaultTimeout time.Duration) (sshCommandSpec, error) {
	spec := sshCommandSpec{
		Host:     strings.TrimSpace(request.Host),
		Username: strings.TrimSpace(request.Username),
		Password: strings.TrimSpace(request.Password),
		Command:  strings.TrimSpace(request.Command),
		Port:     22,
		Timeout:  defaultTimeout,
	}
	if request.Port > 0 {
		spec.Port = int(request.Port)
	}
	if spec.Host == "" || spec.Username == "" || spec.Command == "" || spec.Port <= 0 || spec.Port > 65535 {
		return sshCommandSpec{}, common.ErrInvalidArgument
	}
	privateKey, privateKeyPath, err := sshPrivateKeyInput(request.PrivateKey, request.PrivateKeyPath, request.WorkspaceDeliverableDirectory)
	if err != nil {
		return sshCommandSpec{}, err
	}
	spec.PrivateKey = privateKey
	spec.privateKeyPath = privateKeyPath
	if strings.TrimSpace(spec.Password) == "" && strings.TrimSpace(spec.PrivateKey) == "" && strings.TrimSpace(spec.privateKeyPath) == "" {
		return sshCommandSpec{}, fmt.Errorf("%w: ssh/command requires password or privateKey/privateKeyPath", common.ErrInvalidArgument)
	}
	timeout, err := sshCommandTimeout(request.TimeoutMillis, defaultTimeout)
	if err != nil {
		return sshCommandSpec{}, err
	}
	spec.Timeout = timeout
	return spec, nil
}

func sshPrivateKeyInput(privateKey string, privateKeyPath string, workspaceDeliverableDirectory string) (string, string, error) {
	if value := strings.TrimSpace(privateKey); value != "" {
		return value, "", nil
	}
	path := strings.TrimSpace(privateKeyPath)
	if path == "" {
		return "", "", nil
	}
	if !filepath.IsAbs(path) {
		base := strings.TrimSpace(workspaceDeliverableDirectory)
		if base == "" {
			return "", "", common.ErrInvalidArgument
		}
		path = filepath.Join(base, path)
	}
	return "", path, nil
}

func sshCommandTimeout(timeoutMillis int64, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = 30 * time.Second
	}
	if timeoutMillis == 0 {
		return defaultTimeout, nil
	}
	if timeoutMillis < 0 {
		return 0, common.ErrInvalidArgument
	}
	timeout := time.Duration(timeoutMillis) * time.Millisecond
	if timeout < time.Second {
		return 0, fmt.Errorf("%w: timeoutMillis must be at least 1000", common.ErrInvalidArgument)
	}
	if timeout > 5*time.Minute {
		return 0, fmt.Errorf("%w: timeoutMillis must not exceed 300000", common.ErrInvalidArgument)
	}
	return timeout, nil
}

type sshCommandResult struct {
	Stdout          string
	Stderr          string
	ExitCode        int
	StdoutTruncated bool
	StderrTruncated bool
	StdoutBytes     int
	StderrBytes     int
}

func executeSSHCommand(ctx context.Context, spec sshCommandSpec) (sshCommandResult, error) {
	commandName, args, cleanup, env, err := sshShellCommand(spec)
	if err != nil {
		return sshCommandResult{}, err
	}
	defer cleanup()
	cmd := exec.CommandContext(ctx, commandName, args...)
	cmd.Env = env
	stdout := newLimitedOutputBuffer(MaxCommandOutputBytes)
	stderr := newLimitedOutputBuffer(MaxCommandOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	result := sshCommandResult{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		ExitCode:        exitCode,
		StdoutTruncated: stdout.Truncated(),
		StderrTruncated: stderr.Truncated(),
		StdoutBytes:     stdout.TotalBytes(),
		StderrBytes:     stderr.TotalBytes(),
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, err
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

func sshShellCommand(spec sshCommandSpec) (string, []string, func(), []string, error) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("%w: ssh command is not available", common.ErrInvalidArgument)
	}
	cleanup := func() {}
	env := os.Environ()
	args := []string{
		"-p", strconv.Itoa(spec.Port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=" + strconv.Itoa(int(spec.Timeout.Seconds())),
		"-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=1",
	}
	if keyPath := strings.TrimSpace(spec.privateKeyPath); keyPath != "" {
		args = append(args, "-i", keyPath, "-o", "BatchMode=yes")
	} else if key := strings.TrimSpace(spec.PrivateKey); key != "" {
		dir, err := os.MkdirTemp("", "workorch-ssh-*")
		if err != nil {
			return "", nil, nil, nil, err
		}
		cleanup = func() { _ = os.RemoveAll(dir) }
		keyPath := filepath.Join(dir, "identity")
		if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
			cleanup()
			return "", nil, nil, nil, err
		}
		args = append(args, "-i", keyPath, "-o", "BatchMode=yes")
	} else if password := strings.TrimSpace(spec.Password); password != "" {
		setsidPath, err := exec.LookPath("setsid")
		if err != nil {
			return "", nil, nil, nil, fmt.Errorf("%w: password SSH requires setsid and SSH_ASKPASS support", common.ErrInvalidArgument)
		}
		dir, err := os.MkdirTemp("", "workorch-ssh-askpass-*")
		if err != nil {
			return "", nil, nil, nil, err
		}
		cleanup = func() { _ = os.RemoveAll(dir) }
		askpassPath := filepath.Join(dir, "askpass.sh")
		if err := os.WriteFile(askpassPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$WORKORCH_SSH_PASSWORD\"\n"), 0o700); err != nil {
			cleanup()
			return "", nil, nil, nil, err
		}
		env = append(env,
			"DISPLAY=workorch-ssh",
			"SSH_ASKPASS="+askpassPath,
			"SSH_ASKPASS_REQUIRE=force",
			"WORKORCH_SSH_PASSWORD="+password,
		)
		args = append(args, "-o", "BatchMode=no", "-o", "NumberOfPasswordPrompts=1")
		args = append(args, "-o", "PreferredAuthentications=password,keyboard-interactive")
		args = append(args, spec.Username+"@"+spec.Host, spec.Command)
		return setsidPath, append([]string{sshPath}, args...), cleanup, env, nil
	}
	args = append(args, spec.Username+"@"+spec.Host, spec.Command)
	return sshPath, args, cleanup, env, nil
}
