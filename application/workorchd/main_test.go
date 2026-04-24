package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkorchdDemoOutputContainsAllWorkorchSlices(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run workorchd: %v\noutput:\n%s", err, string(out))
	}

	text := string(out)
	checks := []string{
		"== workorch workorch observable demo ==",
		"workorch runnable slice",
		"workorch runtime-failed-terminal slice",
		"workorch multi-step-completed slice",
		"workorch conditional-branch-completed slice",
		"workorch conditional-branch-review-completed slice",
		"workorch runtime-multi-decision-gate-completed slice",
		"workorch runtime-multi-decision-gate-second-gate-rejected-terminal slice",
		"workorch runtime-decision-blocked-resume slice",
		"workorch runtime-decision-rejected-terminal slice",
		"workorch runtime-approval-blocked-resume slice",
		"workorch runtime-approval-rejected-terminal slice",
		"workorch runtime-mixed-decision-approval-completed slice",
		"workorch runtime-mixed-decision-approval-rejected-terminal slice",
		"workorch decision-blocked-resume slice",
		"workorch decision-rejected-terminal slice",
		"workorch approval-blocked-resume slice",
		"workorch approval-rejected-terminal slice",
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

	runDemo := func() string {
		t.Helper()
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
		return string(out)
	}

	text := runDemo()
	checks := []string{
		"workorch runnable slice",
		"workorch runtime-failed-terminal slice",
		"workorch multi-step-completed slice",
		"workorch conditional-branch-completed slice",
		"workorch conditional-branch-review-completed slice",
		"workorch runtime-multi-decision-gate-completed slice",
		"workorch runtime-multi-decision-gate-second-gate-rejected-terminal slice",
		"workorch runtime-decision-blocked-resume slice",
		"workorch runtime-decision-rejected-terminal slice",
		"workorch runtime-approval-blocked-resume slice",
		"workorch runtime-approval-rejected-terminal slice",
		"workorch runtime-mixed-decision-approval-completed slice",
		"workorch runtime-mixed-decision-approval-rejected-terminal slice",
		"workorch decision-blocked-resume slice",
		"workorch decision-rejected-terminal slice",
		"workorch approval-blocked-resume slice",
		"workorch approval-rejected-terminal slice",
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

	second := runDemo()
	for _, want := range checks {
		if !strings.Contains(second, want) {
			t.Fatalf("second file backend demo output missing %q\noutput:\n%s", want, second)
		}
	}
}

func TestWorkorchdDemoSupportsFileRepositoryBackendWithDefaultDataDir(t *testing.T) {
	tempRoot := t.TempDir()
	defaultDataDir := filepath.Join(tempRoot, "workorchd-data")

	runDemo := func() string {
		t.Helper()
		cmd := exec.Command("go", "run", ".")
		cmd.Dir = "."
		cmd.Env = append(os.Environ(),
			"TMPDIR="+tempRoot,
			"WORKORCH_REPOSITORY_BACKEND=file",
			"WORKORCH_DATA_DIR=",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go run workorchd with default file backend data dir: %v\noutput:\n%s", err, string(out))
		}
		return string(out)
	}

	text := runDemo()
	checks := []string{
		"workorch runnable slice",
		"workorch runtime-failed-terminal slice",
		"workorch multi-step-completed slice",
		"workorch conditional-branch-completed slice",
		"workorch conditional-branch-review-completed slice",
		"workorch runtime-multi-decision-gate-completed slice",
		"workorch runtime-multi-decision-gate-second-gate-rejected-terminal slice",
		"workorch runtime-decision-blocked-resume slice",
		"workorch runtime-decision-rejected-terminal slice",
		"workorch runtime-approval-blocked-resume slice",
		"workorch runtime-approval-rejected-terminal slice",
		"workorch runtime-mixed-decision-approval-completed slice",
		"workorch runtime-mixed-decision-approval-rejected-terminal slice",
		"workorch decision-blocked-resume slice",
		"workorch decision-rejected-terminal slice",
		"workorch approval-blocked-resume slice",
		"workorch approval-rejected-terminal slice",
	}
	for _, want := range checks {
		if !strings.Contains(text, want) {
			t.Fatalf("default file backend demo output missing %q\noutput:\n%s", want, text)
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
		if _, err := os.Stat(filepath.Join(defaultDataDir, name)); err != nil {
			t.Fatalf("expected persisted file %s in default data dir: %v", name, err)
		}
	}

	second := runDemo()
	for _, want := range checks {
		if !strings.Contains(second, want) {
			t.Fatalf("second default file backend demo output missing %q\noutput:\n%s", want, second)
		}
	}
}

func TestWorkorchdDemoFailsWithUnsupportedRepositoryBackend(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"WORKORCH_REPOSITORY_BACKEND=unsupported",
		"WORKORCH_DATA_DIR="+t.TempDir(),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected go run workorchd to fail\noutput:\n%s", string(out))
	}

	text := string(out)
	checks := []string{
		"invalid argument",
		"unsupported repository backend",
	}
	for _, want := range checks {
		if !strings.Contains(text, want) {
			t.Fatalf("unsupported backend output missing %q\noutput:\n%s", want, text)
		}
	}
}
