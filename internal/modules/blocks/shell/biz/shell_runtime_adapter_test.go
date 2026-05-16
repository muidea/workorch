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
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeAdapter_ExecuteRuntimeStepRunsCommandInWorkspaceDirectory(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
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
	if strings.TrimSpace(stdout) != deliverableDirectory {
		t.Fatalf("stdout = %q, want deliverable directory %q", stdout, deliverableDirectory)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepUsesDefaultCommandTimeout(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "pwd",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["timeoutMillis"] != int64(DefaultCommandTimeout/time.Millisecond) {
		t.Fatalf("timeoutMillis = %v, want %d", result.Output["timeoutMillis"], DefaultCommandTimeout/time.Millisecond)
	}
}

func TestNewRuntimeAdapterFromTimeoutString(t *testing.T) {
	adapter, err := NewRuntimeAdapterFromTimeoutString("90s")
	if err != nil {
		t.Fatalf("NewRuntimeAdapterFromTimeoutString: %v", err)
	}
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "pwd",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["timeoutMillis"] != int64(90000) {
		t.Fatalf("timeoutMillis = %v, want 90000", result.Output["timeoutMillis"])
	}
}

func TestNewRuntimeAdapterFromTimeoutStringRejectsOutOfRange(t *testing.T) {
	if _, err := NewRuntimeAdapterFromTimeoutString("500ms"); err == nil {
		t.Fatal("NewRuntimeAdapterFromTimeoutString err = nil, want invalid argument")
	}
	if _, err := NewRuntimeAdapterFromTimeoutString("11m"); err == nil {
		t.Fatal("NewRuntimeAdapterFromTimeoutString err = nil, want invalid argument")
	}
}

func TestNewRuntimeAdapterFromTimeoutStringAllowsTenMinutes(t *testing.T) {
	adapter, err := NewRuntimeAdapterFromTimeoutString("10m")
	if err != nil {
		t.Fatalf("NewRuntimeAdapterFromTimeoutString: %v", err)
	}
	timeout, err := shellCommandTimeout(nil, adapter.timeout)
	if err != nil {
		t.Fatalf("shellCommandTimeout: %v", err)
	}
	if timeout != 10*time.Minute {
		t.Fatalf("timeout = %s, want 10m", timeout)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepCreatesMissingWorkspaceDirectory(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := filepath.Join(t.TempDir(), "missing", "workspace")

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
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
	info, err := os.Stat(deliverableDirectory)
	if err != nil {
		t.Fatalf("stat deliverable directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("deliverableDirectory is not a directory")
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReturnsOutputForNonZeroExit(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
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

func TestRuntimeAdapterAppliesConfiguredZeroWorkEvidencePolicy(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "printf 'testing: warning: no tests to run\\nPASS\\n'"},
			"evidencePolicies": []map[string]any{
				{
					"appliesTo":         "shell/command",
					"commandName":       "sh",
					"argsContain":       []string{"-c"},
					"zeroWorkMarkers":   []string{"no tests to run"},
					"qualityOnZeroWork": string(common.EvidenceQualityInconclusive),
					"evidenceReason":    "configured zero-work marker matched",
					"evidenceTags":      []string{"test", "zero_work"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["evidenceQuality"] != string(common.EvidenceQualityInconclusive) ||
		result.Output["zeroWork"] != true ||
		result.Output["evidenceReason"] != "configured zero-work marker matched" {
		t.Fatalf("output = %+v, want configured inconclusive zero-work evidence", result.Output)
	}
}

func TestSanitizeSuccessfulCommandStderrStripsCurlProgressMeter(t *testing.T) {
	progress := "  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current\n" +
		"                                 Dload  Upload   Total   Spent    Left  Speed\n" +
		"\r100    10  100    10    0     0     33      0 --:--:-- --:--:-- --:--:--    33\n"
	got := sanitizeSuccessfulCommandStderr("curl", []string{"wttr.in/Hangzhou?format=%C+%t"}, 0, "Clear +21C\n", progress)
	if got != "" {
		t.Fatalf("stderr = %q, want curl progress meter stripped", got)
	}
}

func TestSanitizeSuccessfulCommandStderrPreservesRealCurlWarnings(t *testing.T) {
	progress := "  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current\n" +
		"curl: warning: example warning\n" +
		"\r100    10  100    10    0     0     33      0 --:--:-- --:--:-- --:--:--    33\n"
	got := sanitizeSuccessfulCommandStderr("sh", []string{"-c", "curl http://example.test"}, 0, "ok\n", progress)
	if got != "curl: warning: example warning" {
		t.Fatalf("stderr = %q, want non-progress curl stderr preserved", got)
	}
}

func TestSanitizeSuccessfulCommandStderrPreservesFailedCommandStderr(t *testing.T) {
	progress := "  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current\ncurl: (6) Could not resolve host\n"
	got := sanitizeSuccessfulCommandStderr("curl", []string{"http://missing.invalid"}, 6, "", progress)
	if got != progress {
		t.Fatalf("stderr = %q, want failed command stderr unchanged", got)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReturnsTimeoutOutput(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(10 * time.Millisecond)
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
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
	if result.Output["workingDir"] != deliverableDirectory {
		t.Fatalf("workingDir = %v, want %s", result.Output["workingDir"], deliverableDirectory)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepUsesInputTimeoutMillis(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(time.Second)
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
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
		WorkspaceDeliverableDirectory: t.TempDir(),
		Kind:                          "shell/command",
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
				WorkspaceDeliverableDirectory: t.TempDir(),
				Kind:                          "shell/command",
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
