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
	StdoutHash      string
	StderrHash      string
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
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	result := sshCommandResult{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		StdoutHash:      stdout.ContentHash(),
		StderrHash:      stderr.ContentHash(),
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
	dir, err := os.MkdirTemp("", "workorch-ssh-owner-*")
	if err != nil {
		return "", nil, nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	knownHostsPath := filepath.Join(dir, "known_hosts")
	if len(spec.KnownHosts) == 0 {
		cleanup()
		return "", nil, nil, nil, fmt.Errorf("%w: SSH owner has no captured host identity", common.ErrInvalidArgument)
	}
	if err := os.WriteFile(knownHostsPath, spec.KnownHosts, 0o600); err != nil {
		cleanup()
		return "", nil, nil, nil, err
	}
	env := sanitizedSSHEnvironment(os.Environ())
	args := []string{
		"-F", "/dev/null",
		"-p", strconv.Itoa(spec.Port),
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + knownHostsPath,
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "IdentitiesOnly=yes",
		"-o", "ConnectTimeout=" + strconv.Itoa(int(spec.Timeout.Seconds())),
		"-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=1",
	}
	if alias := strings.TrimSpace(spec.HostKeyAlias); alias != "" {
		args = append(args, "-o", "HostKeyAlias="+alias)
	}
	if key := spec.PrivateKey; key != "" {
		keyPath := filepath.Join(dir, "identity")
		if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
			cleanup()
			return "", nil, nil, nil, err
		}
		args = append(args, "-i", keyPath, "-o", "BatchMode=yes", "-o", "PreferredAuthentications=publickey")
	} else if password := spec.Password; password != "" {
		setsidPath, err := exec.LookPath("setsid")
		if err != nil {
			cleanup()
			return "", nil, nil, nil, fmt.Errorf("%w: password SSH requires setsid and SSH_ASKPASS support", common.ErrInvalidArgument)
		}
		askpassPath := filepath.Join(dir, "askpass.sh")
		passwordPath := filepath.Join(dir, "password")
		if err := os.WriteFile(passwordPath, []byte(password), 0o600); err != nil {
			cleanup()
			return "", nil, nil, nil, err
		}
		if err := os.WriteFile(askpassPath, []byte("#!/bin/sh\n/bin/cat -- \"$WORKORCH_SSH_PASSWORD_FILE\"\nprintf '\\n'\n"), 0o700); err != nil {
			cleanup()
			return "", nil, nil, nil, err
		}
		env = append(env,
			"DISPLAY=workorch-ssh",
			"SSH_ASKPASS="+askpassPath,
			"SSH_ASKPASS_REQUIRE=force",
			"WORKORCH_SSH_PASSWORD_FILE="+passwordPath,
		)
		args = append(args, "-o", "BatchMode=no", "-o", "NumberOfPasswordPrompts=1")
		args = append(args, "-o", "PreferredAuthentications=password,keyboard-interactive")
		args = append(args, spec.Username+"@"+spec.Host, spec.Command)
		return setsidPath, append([]string{sshPath}, args...), cleanup, env, nil
	}
	if spec.PrivateKey == "" && spec.Password == "" {
		cleanup()
		return "", nil, nil, nil, fmt.Errorf("%w: SSH owner credential has no material", common.ErrInvalidArgument)
	}
	args = append(args, spec.Username+"@"+spec.Host, spec.Command)
	return sshPath, args, cleanup, env, nil
}

func sanitizedSSHEnvironment(values []string) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		name := value
		if index := strings.IndexByte(name, '='); index >= 0 {
			name = name[:index]
		}
		switch name {
		case "DISPLAY", "SSH_ASKPASS", "SSH_ASKPASS_REQUIRE", "WORKORCH_SSH_PASSWORD", "WORKORCH_SSH_PASSWORD_FILE":
			continue
		}
		ret = append(ret, value)
	}
	return ret
}
