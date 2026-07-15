package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteGoTestArgvMatchesRecordedParameters(t *testing.T) {
	root := "/home/rangh/aispace/workorch/.workorch-workspaces/ws-09c99cd1ce52f6b1"
	if _, err := os.Stat(root); err != nil {
		t.Skip(err)
	}
	// Create a tiny isolated module so package conflict does not mask argv issues.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "cmd/proxy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd/proxy/main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd/proxy/main_test.go"), []byte("package main\nimport \"testing\"\nfunc TestHealthz(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: dir,
		Command:                       "go",
		Args:                          []string{"test", "./cmd/proxy/", "-run", "TestHealthz", "-v"},
	})
	t.Logf("status=%s code=%s msg=%q err=%v stage=%s actual=%s norm=%s stdout=%q stderr=%q",
		result.Status, result.FailureCode, result.FailureMessage, err, result.Stage, result.ActualCommand, result.CommandNormalization, result.Stdout, result.Stderr)
	if err != nil && strings.Contains(result.FailureMessage, "flag provided but not defined: -run") {
		t.Fatalf("argv lost test subcommand: %s", result.FailureMessage)
	}
	// Package with tests should not fail with top-level -run flag error.
	if strings.Contains(result.FailureMessage, "flag provided but not defined") {
		t.Fatalf("unexpected top-level flag error: %s", result.FailureMessage)
	}
}

func TestDirectExecGoWithSameArgs(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n\ngo 1.22\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "cmd/proxy"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "cmd/proxy/main.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "cmd/proxy/main_test.go"), []byte("package main\nimport \"testing\"\nfunc TestHealthz(t *testing.T) {}\n"), 0o644)
	cmd := exec.Command("go", "test", "./cmd/proxy/", "-run", "TestHealthz", "-v")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	t.Logf("direct err=%v out=%s", err, string(out))
	if err != nil && strings.Contains(string(out), "flag provided but not defined: -run") {
		t.Fatal(string(out))
	}
}
