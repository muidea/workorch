package appservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunAllPhase1FromEnvWithFileBackend(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "file")
	t.Setenv("WORKORCH_DATA_DIR", dataRoot)

	slices, err := RunAllPhase1FromEnv(context.Background())
	if err != nil {
		t.Fatalf("RunAllPhase1FromEnv: %v", err)
	}
	if len(slices) != 4 {
		t.Fatalf("len(slices) = %d, want 4", len(slices))
	}

	for _, name := range []string{
		"workspace.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	} {
		if _, err := os.Stat(filepath.Join(dataRoot, name)); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

func TestRunAllPhase1FromEnvWithFileBackendRemainsStableAcrossReloads(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "file")
	t.Setenv("WORKORCH_DATA_DIR", dataRoot)

	first, err := RunAllPhase1FromEnv(context.Background())
	if err != nil {
		t.Fatalf("first RunAllPhase1FromEnv: %v", err)
	}
	second, err := RunAllPhase1FromEnv(context.Background())
	if err != nil {
		t.Fatalf("second RunAllPhase1FromEnv: %v", err)
	}

	assertPhase1SlicesStableAfterReload(t, first, second)
}

func assertPhase1SlicesStableAfterReload(t *testing.T, first []Slice, second []Slice) {
	t.Helper()

	if len(first) != len(second) {
		t.Fatalf("slice count mismatch after reload: first=%d second=%d", len(first), len(second))
	}

	for idx := range first {
		assertPhase1SliceStableAfterReload(t, idx, first[idx], second[idx])
	}
}

func assertPhase1SliceStableAfterReload(t *testing.T, index int, first Slice, second Slice) {
	t.Helper()

	if first.Title != second.Title {
		t.Fatalf("slice %d title mismatch after reload: first=%q second=%q", index, first.Title, second.Title)
	}
	if first.Snapshot == nil || second.Snapshot == nil {
		t.Fatalf("slice %q snapshot nil after reload: first=%v second=%v", first.Title, first.Snapshot == nil, second.Snapshot == nil)
	}

	if first.Snapshot.Slice != second.Snapshot.Slice {
		t.Fatalf("slice %q snapshot slice mismatch after reload: first=%q second=%q", first.Title, first.Snapshot.Slice, second.Snapshot.Slice)
	}
	if first.Snapshot.Workspace == nil || second.Snapshot.Workspace == nil {
		t.Fatalf("slice %q workspace snapshot nil after reload: first=%v second=%v", first.Title, first.Snapshot.Workspace == nil, second.Snapshot.Workspace == nil)
	}
	if *first.Snapshot.Workspace != *second.Snapshot.Workspace {
		t.Fatalf("slice %q workspace mismatch after reload: first=%+v second=%+v", first.Title, *first.Snapshot.Workspace, *second.Snapshot.Workspace)
	}
	if first.Snapshot.Run == nil || second.Snapshot.Run == nil {
		t.Fatalf("slice %q run snapshot nil after reload: first=%v second=%v", first.Title, first.Snapshot.Run == nil, second.Snapshot.Run == nil)
	}
	if *first.Snapshot.Run != *second.Snapshot.Run {
		t.Fatalf("slice %q run mismatch after reload: first=%+v second=%+v", first.Title, *first.Snapshot.Run, *second.Snapshot.Run)
	}
	if len(first.Snapshot.Observed) != len(second.Snapshot.Observed) {
		t.Fatalf("slice %q observed count mismatch after reload: first=%d second=%d", first.Title, len(first.Snapshot.Observed), len(second.Snapshot.Observed))
	}
	for idx := range first.Snapshot.Observed {
		if first.Snapshot.Observed[idx].Topic != second.Snapshot.Observed[idx].Topic {
			t.Fatalf("slice %q observed topic[%d] mismatch after reload: first=%s second=%s", first.Title, idx, first.Snapshot.Observed[idx].Topic, second.Snapshot.Observed[idx].Topic)
		}
	}
}
