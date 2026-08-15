package git

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

func (a *RuntimeAdapter) runGit(ctx context.Context, repoRoot string, timeout time.Duration, limit int64, args ...string) (gitCommandResult, error) {
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	startedAt := time.Now().UTC()
	cmd := exec.CommandContext(runCtx, "git", args...)
	cmd.Dir = repoRoot
	cmd.Env = a.filteredEnv()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	duration := time.Since(startedAt)
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	out, outTruncated := truncateString(stdout.String(), limit)
	errText, errTruncated := truncateString(stderr.String(), maxOutputBytes)
	result := gitCommandResult{
		Stdout:         out,
		Stderr:         errText,
		ExitCode:       exitCode,
		DurationMillis: duration.Milliseconds(),
		Truncated:      outTruncated || errTruncated,
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return result, context.DeadlineExceeded
	}
	return result, err
}

func (a *RuntimeAdapter) gitOutput(ctx context.Context, repoRoot string, timeout time.Duration, args ...string) (string, error) {
	result, err := a.runGit(ctx, repoRoot, timeout, maxOutputBytes, args...)
	if err != nil {
		return result.Stdout, err
	}
	return result.Stdout, nil
}

func (a *RuntimeAdapter) filteredEnv() []string {
	if a == nil || len(a.envAllowlist) == 0 {
		return []string{"GIT_TERMINAL_PROMPT=0"}
	}
	out := make([]string, 0, len(a.envAllowlist)+1)
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, allowed := a.envAllowlist[key]; allowed {
			out = append(out, item)
		}
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0")
	return out
}
