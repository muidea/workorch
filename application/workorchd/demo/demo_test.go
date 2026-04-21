package demo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/application/workorchd/bootstrap"
)

func TestRunAllWithMemoryRuntimeReturnsExpectedSlices(t *testing.T) {
	rt := bootstrap.NewWithBackend("memory", "")

	slices, err := runAll(context.Background(), rt)
	if err != nil {
		t.Fatalf("runAll memory: %v", err)
	}

	if len(slices) != 4 {
		t.Fatalf("expected 4 slices, got %d", len(slices))
	}

	wantTitles := []string{
		"phase1 runnable slice",
		"phase1 decision-blocked-resume slice",
		"phase1 approval-blocked-resume slice",
		"phase1 approval-rejected-terminal slice",
	}
	for idx, want := range wantTitles {
		if slices[idx].Title != want {
			t.Fatalf("slice %d title mismatch, want %q got %q", idx, want, slices[idx].Title)
		}
		if slices[idx].Snapshot == nil {
			t.Fatalf("slice %d snapshot is nil", idx)
		}
	}

	if got := slices[0].Snapshot.Run.State; got != "completed" {
		t.Fatalf("runnable slice run state mismatch: %s", got)
	}
	if got := slices[1].Snapshot.Run.State; got != "completed" {
		t.Fatalf("decision blocked/resume run state mismatch: %s", got)
	}
	if got := slices[2].Snapshot.Run.State; got != "completed" {
		t.Fatalf("approval blocked/resume run state mismatch: %s", got)
	}
	if got := slices[3].Snapshot.Run.State; got != "failed" {
		t.Fatalf("approval rejected terminal run state mismatch: %s", got)
	}

	if slices[1].Snapshot.Blocking == nil || slices[1].Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("decision blocked/resume slice missing decision ticket snapshot")
	}
	if slices[2].Snapshot.Blocking == nil || slices[2].Snapshot.Blocking.Approval == nil {
		t.Fatal("approval blocked/resume slice missing approval snapshot")
	}
	if slices[0].Snapshot.Step == nil || slices[0].Snapshot.Step.State != "completed" {
		t.Fatalf("runnable slice step mismatch: %+v", slices[0].Snapshot.Step)
	}
	if slices[3].Snapshot.Step != nil {
		t.Fatalf("approval rejected terminal slice should not dispatch runtime step, got %+v", slices[3].Snapshot.Step)
	}
}

func TestRunAllWithFileRuntimePersistsExpectedFiles(t *testing.T) {
	dataDir := t.TempDir()
	rt := bootstrap.NewWithBackend("file", dataDir)

	slices, err := runAll(context.Background(), rt)
	if err != nil {
		t.Fatalf("runAll file: %v", err)
	}
	if len(slices) != 4 {
		t.Fatalf("expected 4 slices, got %d", len(slices))
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

	if got := slices[3].Snapshot.Run.State; got != "failed" {
		t.Fatalf("expected approval rejected terminal slice to stay failed, got %s", got)
	}
}
