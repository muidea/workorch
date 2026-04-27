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

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeAdapter struct {
	timeout time.Duration
}

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

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	spec, err := sshCommandSpecFromInput(request.Input, request.WorkspaceWorkingDirectory, a.defaultTimeout())
	if err != nil {
		return runtimemodel.StepExecutionResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	startedAt := time.Now().UTC()
	stdout, stderr, exitCode, err := executeSSHCommand(runCtx, spec)
	duration := time.Since(startedAt)
	output := map[string]any{
		"status":         "completed",
		"host":           spec.Host,
		"port":           spec.Port,
		"username":       spec.Username,
		"command":        spec.Command,
		"stdout":         stdout,
		"stderr":         stderr,
		"exitCode":       exitCode,
		"durationMillis": duration.Milliseconds(),
		"timeoutMillis":  spec.Timeout.Milliseconds(),
		"authMethod":     spec.authMethod(),
	}
	if runCtx.Err() == context.DeadlineExceeded {
		output["status"] = "timeout"
		return runtimemodel.StepExecutionResult{Output: output}, context.DeadlineExceeded
	}
	if err != nil {
		output["status"] = "failed"
		output["error"] = err.Error()
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
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
	if strings.TrimSpace(s.PrivateKey) != "" {
		return "private_key"
	}
	if strings.TrimSpace(s.Password) != "" {
		return "password"
	}
	return ""
}

func sshCommandSpecFromInput(input map[string]any, workspaceWorkingDirectory string, defaultTimeout time.Duration) (sshCommandSpec, error) {
	if input == nil {
		return sshCommandSpec{}, common.ErrInvalidArgument
	}
	spec := sshCommandSpec{
		Host:     stringInput(input, "host"),
		Username: stringInput(input, "username"),
		Password: stringInput(input, "password"),
		Command:  stringInput(input, "command"),
		Port:     22,
		Timeout:  defaultTimeout,
	}
	if port, ok, err := numericInput(input["port"]); err != nil {
		return sshCommandSpec{}, err
	} else if ok {
		spec.Port = int(port)
	}
	if spec.Host == "" || spec.Username == "" || spec.Command == "" || spec.Port <= 0 || spec.Port > 65535 {
		return sshCommandSpec{}, common.ErrInvalidArgument
	}
	privateKey, privateKeyPath, err := sshPrivateKeyInput(input, workspaceWorkingDirectory)
	if err != nil {
		return sshCommandSpec{}, err
	}
	spec.PrivateKey = privateKey
	spec.privateKeyPath = privateKeyPath
	if strings.TrimSpace(spec.Password) == "" && strings.TrimSpace(spec.PrivateKey) == "" && strings.TrimSpace(spec.privateKeyPath) == "" {
		return sshCommandSpec{}, fmt.Errorf("%w: ssh/command requires password or privateKey/privateKeyPath", common.ErrInvalidArgument)
	}
	timeout, err := sshCommandTimeout(input, defaultTimeout)
	if err != nil {
		return sshCommandSpec{}, err
	}
	spec.Timeout = timeout
	return spec, nil
}

func stringInput(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func sshPrivateKeyInput(input map[string]any, workspaceWorkingDirectory string) (string, string, error) {
	if value := stringInput(input, "privateKey"); value != "" {
		return value, "", nil
	}
	path := stringInput(input, "privateKeyPath")
	if path == "" {
		return "", "", nil
	}
	if !filepath.IsAbs(path) {
		base := strings.TrimSpace(workspaceWorkingDirectory)
		if base == "" {
			return "", "", common.ErrInvalidArgument
		}
		path = filepath.Join(base, path)
	}
	return "", path, nil
}

func sshCommandTimeout(input map[string]any, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = 30 * time.Second
	}
	if input == nil || input["timeoutMillis"] == nil {
		return defaultTimeout, nil
	}
	value, ok, err := numericInput(input["timeoutMillis"])
	if err != nil {
		return 0, err
	}
	if !ok || value <= 0 {
		return 0, common.ErrInvalidArgument
	}
	timeout := time.Duration(value) * time.Millisecond
	if timeout < time.Second {
		return 0, fmt.Errorf("%w: timeoutMillis must be at least 1000", common.ErrInvalidArgument)
	}
	if timeout > 5*time.Minute {
		return 0, fmt.Errorf("%w: timeoutMillis must not exceed 300000", common.ErrInvalidArgument)
	}
	return timeout, nil
}

func numericInput(input any) (int64, bool, error) {
	switch typed := input.(type) {
	case nil:
		return 0, false, nil
	case int:
		return int64(typed), true, nil
	case int64:
		return typed, true, nil
	case int32:
		return int64(typed), true, nil
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false, common.ErrInvalidArgument
		}
		return int64(typed), true, nil
	case float32:
		if typed != float32(int64(typed)) {
			return 0, false, common.ErrInvalidArgument
		}
		return int64(typed), true, nil
	case string:
		value := strings.TrimSpace(typed)
		if value == "" {
			return 0, false, nil
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false, common.ErrInvalidArgument
		}
		return parsed, true, nil
	default:
		return 0, false, common.ErrInvalidArgument
	}
}

func executeSSHCommand(ctx context.Context, spec sshCommandSpec) (string, string, int, error) {
	commandName, args, cleanup, env, err := sshShellCommand(spec)
	if err != nil {
		return "", "", 0, err
	}
	defer cleanup()
	cmd := exec.CommandContext(ctx, commandName, args...)
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		return stdout.String(), stderr.String(), exitCode, ctx.Err()
	}
	return stdout.String(), stderr.String(), exitCode, err
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
