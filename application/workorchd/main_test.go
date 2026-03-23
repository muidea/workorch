package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkorchdDemoOutputContainsAllPhase1Slices(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run workorchd: %v\noutput:\n%s", err, string(out))
	}

	text := string(out)
	checks := []string{
		"== workorch phase1 observable demo ==",
		"phase1 runnable slice",
		"phase1 decision-blocked-resume slice",
		"phase1 approval-blocked-resume slice",
		"phase1 approval-rejected-terminal slice",
		"/workspace/snapshot",
		"/run/snapshot",
		"/step/snapshot",
		"/decision/ticket/snapshot",
		"/approval/snapshot",
	}
	for _, want := range checks {
		if !strings.Contains(text, want) {
			t.Fatalf("demo output missing %q\noutput:\n%s", want, text)
		}
	}
}

func TestWorkorchdDemoSupportsFileRepositoryBackend(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "workorch-data")

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"WORKORCH_REPOSITORY_BACKEND=file",
		"WORKORCH_DATA_DIR="+dataDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run workorchd with file backend: %v\noutput:\n%s", err, string(out))
	}

	text := string(out)
	checks := []string{
		"phase1 runnable slice",
		"phase1 decision-blocked-resume slice",
		"phase1 approval-blocked-resume slice",
	}
	for _, want := range checks {
		if !strings.Contains(text, want) {
			t.Fatalf("file backend demo output missing %q\noutput:\n%s", want, text)
		}
	}

	files := []string{
		"workspace.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	}
	for _, name := range files {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
			t.Fatalf("expected persisted file %s: %v", name, err)
		}
	}
}
