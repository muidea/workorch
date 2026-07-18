package ssh

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeAdapterExecutesPasswordSSHThroughShellCommand(t *testing.T) {
	toolsDir := t.TempDir()
	captureFile := filepath.Join(toolsDir, "capture.txt")
	writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\n{\n  echo \"args=$*\"\n  echo \"askpass=$SSH_ASKPASS\"\n  echo \"askpass_require=$SSH_ASKPASS_REQUIRE\"\n  echo \"password=$($SSH_ASKPASS)\"\n} > \""+captureFile+"\"\necho remote-output\n")
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := NewRuntimeAdapterWithTimeout(5 * time.Second)
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID:   "ws-ssh",
		RunID:         "run-ssh",
		StepID:        "run-ssh/ssh-1",
		Host:          "192.168.19.231",
		Port:          22,
		Username:      "fedquery",
		Password:      "yuxiang",
		Command:       "hostname; lscpu",
		TimeoutMillis: 5000,
	})
	if err != nil {
		t.Fatalf("execute ssh command: %v", err)
	}
	if result.Status != "completed" ||
		result.AuthMethod != "password" ||
		result.Stdout != "remote-output\n" ||
		result.Command != "hostname; lscpu" {
		t.Fatalf("output = %+v, want completed password ssh command", result)
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
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID:   "ws-ssh",
		RunID:         "run-ssh",
		StepID:        "run-ssh/ssh-1",
		Host:          "example.internal",
		Username:      "fedquery",
		PrivateKey:    "test-private-key",
		Command:       "hostname",
		TimeoutMillis: 5000,
	})
	if err != nil {
		t.Fatalf("execute ssh command: %v", err)
	}
	if result.Status != "completed" ||
		result.AuthMethod != "private_key" ||
		result.Stdout != "key-output\n" {
		t.Fatalf("output = %+v, want completed private key ssh command", result)
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
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID:   "ws-ssh",
		RunID:         "run-ssh",
		StepID:        "run-ssh/ssh-1",
		Host:          "example.internal",
		Username:      "fedquery",
		PrivateKey:    "test-private-key",
		Command:       "large-output",
		TimeoutMillis: 5000,
	})
	if err != nil {
		t.Fatalf("execute ssh command: %v", err)
	}
	if len([]byte(result.Stdout)) != MaxCommandOutputBytes {
		t.Fatalf("stdout bytes = %d, want %d", len([]byte(result.Stdout)), MaxCommandOutputBytes)
	}
	if !result.OutputTruncated || !result.StdoutTruncated {
		t.Fatalf("output = %+v, want stdout truncation metadata", result)
	}
	if result.StdoutBytes != 70000 {
		t.Fatalf("stdoutBytes = %v, want 70000", result.StdoutBytes)
	}
}

func TestRuntimeAdapterRejectsMissingAuthentication(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID: "ws-ssh",
		RunID:       "run-ssh",
		StepID:      "run-ssh/ssh-1",
		Host:        "127.0.0.1",
		Username:    "fedquery",
		Command:     "hostname",
	})
	if err == nil || !strings.Contains(err.Error(), "password or privateKey") {
		t.Fatalf("err = %v, want missing authentication error", err)
	}
}

func TestRuntimeAdapterProbeValidatesLocalPrerequisitesWithoutSSHExecution(t *testing.T) {
	toolsDir := t.TempDir()
	marker := filepath.Join(toolsDir, "ssh-called")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\ntouch \""+marker+"\"\n")
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	adapter := NewRuntimeAdapter()
	if err := adapter.ProbeOperation(RuntimeCommandRequest{
		WorkspaceID: "ws-ssh", RunID: "run-ssh", StepID: "probe/ssh-1",
		Host: "example.internal", Username: "operator", Password: "secret", Command: "hostname", TimeoutMillis: 5000,
	}); err != nil {
		t.Fatalf("probe valid ssh command: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("probe started ssh command: stat err=%v", err)
	}
	if err := adapter.ProbeOperation(RuntimeCommandRequest{
		Host: "example.internal", Username: "operator", PrivateKeyPath: "missing-key", Command: "hostname",
	}); err == nil {
		t.Fatal("probe accepted missing private key path")
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
