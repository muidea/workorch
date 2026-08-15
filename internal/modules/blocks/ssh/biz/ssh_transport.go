package ssh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

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
