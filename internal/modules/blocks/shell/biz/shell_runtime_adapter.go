package shell

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeAdapter struct {
	timeout      time.Duration
	envAllowlist map[string]struct{}
}

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{
		timeout: 5 * time.Second,
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

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	command, args, err := shellCommandSpec(request.Input)
	if err != nil {
		return runtimemodel.StepExecutionResult{}, err
	}
	workingDirectory, err := filepath.Abs(strings.TrimSpace(request.WorkspaceWorkingDirectory))
	if err != nil || workingDirectory == "" {
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		return runtimemodel.StepExecutionResult{}, err
	}

	timeout := a.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startedAt := time.Now().UTC()
	cmd := exec.CommandContext(runCtx, command, args...)
	cmd.Dir = workingDirectory
	cmd.Env = a.filteredEnv()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	duration := time.Since(startedAt)
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	output := map[string]any{
		"status":         "completed",
		"command":        command,
		"args":           args,
		"workingDir":     workingDirectory,
		"stdout":         stdout.String(),
		"stderr":         stderr.String(),
		"exitCode":       exitCode,
		"durationMillis": duration.Milliseconds(),
	}
	if runCtx.Err() == context.DeadlineExceeded {
		output["status"] = "timeout"
		return runtimemodel.StepExecutionResult{Output: output}, context.DeadlineExceeded
	}
	if err != nil {
		output["status"] = "failed"
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	return runtimemodel.StepExecutionResult{Output: output}, nil
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

func shellCommandSpec(input map[string]any) (string, []string, error) {
	if input == nil {
		return "", nil, common.ErrInvalidArgument
	}
	command, _ := input["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, common.ErrInvalidArgument
	}
	args, err := shellCommandArgs(input["args"])
	if err != nil {
		return "", nil, err
	}
	return command, args, nil
}

func shellCommandArgs(input any) ([]string, error) {
	if input == nil {
		return nil, nil
	}
	switch typed := input.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			ret = append(ret, value)
		}
		return ret, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}
