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

const (
	DefaultCommandTimeout = 60 * time.Second
	MaxCommandTimeout     = 10 * time.Minute
)

type RuntimeAdapter struct {
	timeout      time.Duration
	envAllowlist map[string]struct{}
}

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{
		timeout: DefaultCommandTimeout,
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

func NewRuntimeAdapterFromTimeoutString(timeoutValue string) (*RuntimeAdapter, error) {
	adapter := NewRuntimeAdapter()
	timeoutValue = strings.TrimSpace(timeoutValue)
	if timeoutValue == "" {
		return adapter, nil
	}
	timeout, err := time.ParseDuration(timeoutValue)
	if err != nil {
		return nil, fmt.Errorf("%w: runtime.shellCommandTimeout must be a valid duration", common.ErrInvalidArgument)
	}
	if timeout < time.Second {
		return nil, fmt.Errorf("%w: runtime.shellCommandTimeout must be at least 1s", common.ErrInvalidArgument)
	}
	if timeout > MaxCommandTimeout {
		return nil, fmt.Errorf("%w: runtime.shellCommandTimeout must not exceed %s", common.ErrInvalidArgument, MaxCommandTimeout)
	}
	adapter.timeout = timeout
	return adapter, nil
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
	if err := validateShellCommandPolicy(command, args, workingDirectory); err != nil {
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
		"stderr":         sanitizeSuccessfulCommandStderr(command, args, exitCode, stdout.String(), stderr.String()),
		"exitCode":       exitCode,
		"durationMillis": duration.Milliseconds(),
		"timeoutMillis":  timeout.Milliseconds(),
	}
	applyShellEvidencePolicies(request.Input, command, args, output)
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
		defaultTimeout = DefaultCommandTimeout
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
	if timeout > MaxCommandTimeout {
		return 0, fmt.Errorf("%w: timeoutMillis must not exceed %d", common.ErrInvalidArgument, MaxCommandTimeout/time.Millisecond)
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

func validateShellCommandPolicy(command string, args []string, workingDirectory string) error {
	base := filepath.Base(strings.TrimSpace(command))
	if base == "" {
		return common.ErrInvalidArgument
	}
	switch base {
	case "sudo", "su", "shutdown", "reboot", "poweroff", "halt", "mkfs", "mount", "umount":
		return fmt.Errorf("%w: shell command %q is not allowed", common.ErrInvalidArgument, base)
	case "rm":
		if rmCommandIsDangerous(args, workingDirectory) {
			return fmt.Errorf("%w: destructive rm command is not allowed", common.ErrInvalidArgument)
		}
	case "chmod":
		if chmodCommandIsDangerous(args) {
			return fmt.Errorf("%w: chmod 777 is not allowed", common.ErrInvalidArgument)
		}
	case "git":
		if gitCommandIsDangerous(args) {
			return fmt.Errorf("%w: git force push is not allowed", common.ErrInvalidArgument)
		}
	case "sh", "bash", "zsh":
		if shellScriptIsDangerous(args) {
			return fmt.Errorf("%w: destructive shell script is not allowed", common.ErrInvalidArgument)
		}
	}
	return nil
}

func rmCommandIsDangerous(args []string, workingDirectory string) bool {
	recursiveForce := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && strings.Contains(arg, "r") && strings.Contains(arg, "f") {
			recursiveForce = true
			break
		}
	}
	if !recursiveForce {
		return false
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		target := strings.TrimSpace(arg)
		switch target {
		case "", "/", ".", "..", "*", ".git", "./.git":
			return true
		}
		if filepath.IsAbs(target) {
			cleanTarget := filepath.Clean(target)
			cleanWorking := filepath.Clean(workingDirectory)
			if cleanTarget == cleanWorking || !strings.HasPrefix(cleanTarget, cleanWorking+string(os.PathSeparator)) {
				return true
			}
		}
		if strings.Contains(target, "..") {
			return true
		}
	}
	return false
}

func chmodCommandIsDangerous(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == "777" {
			return true
		}
	}
	return false
}

func gitCommandIsDangerous(args []string) bool {
	if len(args) < 2 || args[0] != "push" {
		return false
	}
	for _, arg := range args[1:] {
		if arg == "--force" || arg == "-f" || strings.HasPrefix(arg, "--force-") {
			return true
		}
	}
	return false
}

func shellScriptIsDangerous(args []string) bool {
	for idx, arg := range args {
		if arg != "-c" || idx+1 >= len(args) {
			continue
		}
		script := strings.ToLower(args[idx+1])
		dangerousMarkers := []string{
			"rm -rf /",
			"rm -fr /",
			"rm -rf .git",
			"rm -fr .git",
			"sudo ",
			" shutdown",
			" reboot",
			" poweroff",
			" mkfs",
			"git push --force",
			"git push -f",
			"chmod 777",
		}
		padded := " " + script
		for _, marker := range dangerousMarkers {
			if strings.Contains(padded, marker) {
				return true
			}
		}
	}
	return false
}

func sanitizeSuccessfulCommandStderr(command string, args []string, exitCode int, stdoutText string, stderrText string) string {
	if exitCode != 0 || strings.TrimSpace(stdoutText) == "" || strings.TrimSpace(stderrText) == "" {
		return stderrText
	}
	if !shellCommandMayEmitCurlProgress(command, args) {
		return stderrText
	}
	cleaned, changed := stripCurlProgressMeter(stderrText)
	if !changed {
		return stderrText
	}
	return cleaned
}

func shellCommandMayEmitCurlProgress(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	if base == "curl" {
		return true
	}
	switch base {
	case "sh", "bash", "zsh":
		for idx, arg := range args {
			if arg != "-c" || idx+1 >= len(args) {
				continue
			}
			if shellScriptContainsCurl(args[idx+1]) {
				return true
			}
		}
	}
	return false
}

func shellScriptContainsCurl(script string) bool {
	fields := strings.Fields(strings.TrimSpace(script))
	for _, field := range fields {
		field = strings.Trim(field, "'\"`();|&")
		if filepath.Base(field) == "curl" {
			return true
		}
	}
	return false
}

func stripCurlProgressMeter(stderrText string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(stderrText, "\r", "\n"), "\n")
	ret := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		if isCurlProgressMeterLine(line) {
			changed = true
			continue
		}
		ret = append(ret, line)
	}
	if !changed {
		return stderrText, false
	}
	return strings.TrimSpace(strings.Join(ret, "\n")), true
}

func isCurlProgressMeterLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "% Total") && strings.Contains(trimmed, "% Received") {
		return true
	}
	if strings.Contains(trimmed, "Dload") && strings.Contains(trimmed, "Upload") && strings.Contains(trimmed, "Speed") {
		return true
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 10 {
		return false
	}
	if !allDigits(fields[0]) {
		return false
	}
	return strings.Contains(trimmed, "--:--:--") || strings.Contains(trimmed, "k") || strings.Contains(trimmed, "M")
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
