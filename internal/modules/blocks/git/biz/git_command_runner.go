package git

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

const gitTruncatedOutputMarker = "\n[truncated]"

// boundedGitOutput counts the complete child-process stream while retaining
// only the configured prefix. bytes.Buffer followed by post-process
// truncation would still allocate for unbounded repository output.
type boundedGitOutput struct {
	limit int
	total int64
	buf   bytes.Buffer
}

func newBoundedGitOutput(limit int64) *boundedGitOutput {
	if limit <= 0 {
		limit = maxOutputBytes
	}
	return &boundedGitOutput{limit: int(limit)}
}

func (b *boundedGitOutput) Write(data []byte) (int, error) {
	written := len(data)
	if b == nil {
		return written, nil
	}
	b.total += int64(written)
	if b.buf.Len() >= b.limit {
		return written, nil
	}
	remaining := b.limit - b.buf.Len()
	if len(data) > remaining {
		data = data[:remaining]
	}
	_, _ = b.buf.Write(data)
	return written, nil
}

func (b *boundedGitOutput) String() string {
	if b == nil {
		return ""
	}
	value := b.buf.String()
	if b.Truncated() && b.limit >= 32 {
		value += gitTruncatedOutputMarker
	}
	return value
}

func (b *boundedGitOutput) TotalBytes() int64 {
	if b == nil {
		return 0
	}
	return b.total
}

func (b *boundedGitOutput) Truncated() bool {
	return b != nil && b.total > int64(b.limit)
}

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
	stdout := newBoundedGitOutput(limit)
	stderr := newBoundedGitOutput(maxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	duration := time.Since(startedAt)
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	result := gitCommandResult{
		Stdout:         stdout.String(),
		Stderr:         stderr.String(),
		StdoutBytes:    stdout.TotalBytes(),
		StderrBytes:    stderr.TotalBytes(),
		ExitCode:       exitCode,
		DurationMillis: duration.Milliseconds(),
		Truncated:      stdout.Truncated() || stderr.Truncated(),
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
