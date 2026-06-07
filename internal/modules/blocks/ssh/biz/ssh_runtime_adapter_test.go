package ssh

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterExecutesPasswordSSHThroughShellCommand(t *testing.T) {
	toolsDir := t.TempDir()
	captureFile := filepath.Join(toolsDir, "capture.txt")
	writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\n{\n  echo \"args=$*\"\n  echo \"askpass=$SSH_ASKPASS\"\n  echo \"askpass_require=$SSH_ASKPASS_REQUIRE\"\n  echo \"password=$($SSH_ASKPASS)\"\n} > \""+captureFile+"\"\necho remote-output\n")
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := NewRuntimeAdapterWithTimeout(5 * time.Second)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID: "ws-ssh",
		RunID:       "run-ssh",
		StepID:      "run-ssh/ssh-1",
		Kind:        "ssh/command",
		Input: map[string]any{
			"host":          "192.168.19.231",
			"port":          22,
			"username":      "fedquery",
			"password":      "yuxiang",
			"command":       "hostname; lscpu",
			"timeoutMillis": float64(5000),
		},
	})
	if err != nil {
		t.Fatalf("execute ssh command: %v", err)
	}
	if result.Output["status"] != "completed" ||
		result.Output["authMethod"] != "password" ||
		result.Output["stdout"] != "remote-output\n" ||
		result.Output["command"] != "hostname; lscpu" {
		t.Fatalf("output = %+v, want completed password ssh command", result.Output)
	}
	if _, leaked := result.Output["password"]; leaked {
		t.Fatalf("output leaks password: %+v", result.Output)
	}
	captured := readFile(t, captureFile)
	for _, want := range []string{
		"-p 22",
		"-o BatchMode=no",
		"-o NumberOfPasswordPrompts=1",
		"fedquery@192.168.19.231 hostname; lscpu",
		"askpass_require=force",
		"password=yuxiang",
	} {
		if !strings.Contains(captured, want) {
			t.Fatalf("captured = %q, want %q", captured, want)
		}
	}
}

func TestRuntimeAdapterExecutesPrivateKeySSHThroughShellCommand(t *testing.T) {
	toolsDir := t.TempDir()
	captureFile := filepath.Join(toolsDir, "capture.txt")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\necho \"args=$*\" > \""+captureFile+"\"\necho key-output\n")
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := NewRuntimeAdapterWithTimeout(5 * time.Second)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID: "ws-ssh",
		RunID:       "run-ssh",
		StepID:      "run-ssh/ssh-1",
		Kind:        "ssh/command",
		Input: map[string]any{
			"host":          "example.internal",
			"username":      "fedquery",
			"privateKey":    "test-private-key",
			"command":       "hostname",
			"timeoutMillis": 5000,
		},
	})
	if err != nil {
		t.Fatalf("execute ssh command: %v", err)
	}
	if result.Output["status"] != "completed" ||
		result.Output["authMethod"] != "private_key" ||
		result.Output["stdout"] != "key-output\n" {
		t.Fatalf("output = %+v, want completed private key ssh command", result.Output)
	}
	captured := readFile(t, captureFile)
	for _, want := range []string{"-i ", "-o BatchMode=yes", "fedquery@example.internal hostname"} {
		if !strings.Contains(captured, want) {
			t.Fatalf("captured = %q, want %q", captured, want)
		}
	}
}

func TestRuntimeAdapterBoundsSSHCommandOutput(t *testing.T) {
	toolsDir := t.TempDir()
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\nyes x | head -c 70000\n")
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := NewRuntimeAdapterWithTimeout(5 * time.Second)
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID: "ws-ssh",
		RunID:       "run-ssh",
		StepID:      "run-ssh/ssh-1",
		Kind:        "ssh/command",
		Input: map[string]any{
			"host":          "example.internal",
			"username":      "fedquery",
			"privateKey":    "test-private-key",
			"command":       "large-output",
			"timeoutMillis": 5000,
		},
	})
	if err != nil {
		t.Fatalf("execute ssh command: %v", err)
	}
	stdout, _ := result.Output["stdout"].(string)
	if len([]byte(stdout)) != MaxCommandOutputBytes {
		t.Fatalf("stdout bytes = %d, want %d", len([]byte(stdout)), MaxCommandOutputBytes)
	}
	if result.Output["outputTruncated"] != true || result.Output["stdoutTruncated"] != true {
		t.Fatalf("output = %+v, want stdout truncation metadata", result.Output)
	}
	if result.Output["stdoutBytes"] != 70000 {
		t.Fatalf("stdoutBytes = %v, want 70000", result.Output["stdoutBytes"])
	}
}

func TestRuntimeAdapterRejectsMissingAuthentication(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID: "ws-ssh",
		RunID:       "run-ssh",
		StepID:      "run-ssh/ssh-1",
		Kind:        "ssh/command",
		Input: map[string]any{
			"host":     "127.0.0.1",
			"username": "fedquery",
			"command":  "hostname",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "password or privateKey") {
		t.Fatalf("err = %v, want missing authentication error", err)
	}
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
