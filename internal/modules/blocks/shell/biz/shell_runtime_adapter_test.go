package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func TestRuntimeAdapter_ExecuteRuntimeStepRunsCommandInWorkspaceDirectory(t *testing.T) {
	adapter := NewRuntimeAdapter()
	workingDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceWorkingDirectory: workingDirectory,
		Kind:                      "shell/command",
		Input: map[string]any{
			"command": "pwd",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["exitCode"] != 0 {
		t.Fatalf("exitCode = %v, want 0", result.Output["exitCode"])
	}
	stdout, _ := result.Output["stdout"].(string)
	if strings.TrimSpace(stdout) != workingDirectory {
		t.Fatalf("stdout = %q, want working directory %q", stdout, workingDirectory)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepCreatesMissingWorkspaceDirectory(t *testing.T) {
	adapter := NewRuntimeAdapter()
	workingDirectory := filepath.Join(t.TempDir(), "missing", "workspace")

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceWorkingDirectory: workingDirectory,
		Kind:                      "shell/command",
		Input: map[string]any{
			"command": "pwd",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["exitCode"] != 0 {
		t.Fatalf("exitCode = %v, want 0", result.Output["exitCode"])
	}
	info, err := os.Stat(workingDirectory)
	if err != nil {
		t.Fatalf("stat working directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("workingDirectory is not a directory")
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReturnsOutputForNonZeroExit(t *testing.T) {
	adapter := NewRuntimeAdapter()
	workingDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceWorkingDirectory: workingDirectory,
		Kind:                      "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "echo shell-out; echo shell-err >&2; exit 7"},
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want non-zero exit error")
	}
	if result.Output["status"] != "failed" {
		t.Fatalf("status = %v, want failed", result.Output["status"])
	}
	if result.Output["exitCode"] != 7 {
		t.Fatalf("exitCode = %v, want 7", result.Output["exitCode"])
	}
	stdout, _ := result.Output["stdout"].(string)
	stderr, _ := result.Output["stderr"].(string)
	if !strings.Contains(stdout, "shell-out") || !strings.Contains(stderr, "shell-err") {
		t.Fatalf("stdout/stderr = %q/%q, want captured command output", stdout, stderr)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReturnsTimeoutOutput(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(10 * time.Millisecond)
	workingDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceWorkingDirectory: workingDirectory,
		Kind:                      "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "sleep 1"},
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecuteRuntimeStep err = %v, want deadline exceeded", err)
	}
	if result.Output["status"] != "timeout" {
		t.Fatalf("status = %v, want timeout", result.Output["status"])
	}
	if result.Output["workingDir"] != workingDirectory {
		t.Fatalf("workingDir = %v, want %s", result.Output["workingDir"], workingDirectory)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepUsesInputTimeoutMillis(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(time.Second)
	workingDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceWorkingDirectory: workingDirectory,
		Kind:                      "shell/command",
		Input: map[string]any{
			"command":       "sh",
			"args":          []any{"-c", "sleep 0.2; echo done"},
			"timeoutMillis": 2000,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["status"] != "completed" {
		t.Fatalf("status = %v, want completed", result.Output["status"])
	}
	if result.Output["timeoutMillis"] != int64(2000) {
		t.Fatalf("timeoutMillis = %v, want 2000", result.Output["timeoutMillis"])
	}
	stdout, _ := result.Output["stdout"].(string)
	if strings.TrimSpace(stdout) != "done" {
		t.Fatalf("stdout = %q, want done", stdout)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRejectsInvalidInputTimeoutMillis(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceWorkingDirectory: t.TempDir(),
		Kind:                      "shell/command",
		Input: map[string]any{
			"command":       "pwd",
			"timeoutMillis": 500,
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want invalid argument")
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRejectsDangerousCommands(t *testing.T) {
	adapter := NewRuntimeAdapter()
	cases := []struct {
		name    string
		command string
		args    []any
	}{
		{name: "sudo", command: "sudo", args: []any{"true"}},
		{name: "rm root", command: "rm", args: []any{"-rf", "/"}},
		{name: "rm git", command: "rm", args: []any{"-rf", ".git"}},
		{name: "chmod 777", command: "chmod", args: []any{"777", "file.txt"}},
		{name: "git force push", command: "git", args: []any{"push", "--force"}},
		{name: "shell script", command: "sh", args: []any{"-c", "rm -rf /tmp/project/.git"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
				WorkspaceWorkingDirectory: t.TempDir(),
				Kind:                      "shell/command",
				Input: map[string]any{
					"command": tc.command,
					"args":    tc.args,
				},
			})
			if err == nil {
				t.Fatal("ExecuteRuntimeStep err = nil, want invalid argument")
			}
		})
	}
}
