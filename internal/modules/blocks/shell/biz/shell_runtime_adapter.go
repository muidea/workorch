package shell

import (
	"bytes"
	"context"
	"fmt"
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

	timeout, err := shellCommandTimeout(request.Input, a.timeout)
	if err != nil {
		return runtimemodel.StepExecutionResult{}, err
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
		"timeoutMillis":  timeout.Milliseconds(),
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

func shellCommandTimeout(input map[string]any, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = 5 * time.Second
	}
	if input == nil || input["timeoutMillis"] == nil {
		return defaultTimeout, nil
	}
	value, ok := numericMillis(input["timeoutMillis"])
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

func numericMillis(input any) (int64, bool) {
	switch typed := input.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		if typed != float32(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	default:
		return 0, false
	}
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
