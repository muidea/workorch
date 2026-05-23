package runtimepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafePathKeepsReadableStructuredSegments(t *testing.T) {
	if got := SafePath("run-1/graph-002/007-run-go-test"); got != filepath.Join("run-1", "graph-002", "007-run-go-test") {
		t.Fatalf("SafePath = %q, want readable structured path", got)
	}
	if got := SafePath("../bad/ /中文"); got != filepath.Join("bad", "item") {
		t.Fatalf("SafePath unsafe = %q, want cleaned path", got)
	}
}

func TestSequenceScopedPathUsesScopeSequenceAndStableExistingPath(t *testing.T) {
	root := t.TempDir()
	if got := SequenceScopedPath(root, "run-001", "run-001/step-1"); got != "run-001/001-step-1" {
		t.Fatalf("first path = %q, want first sequence", got)
	}
	if err := os.MkdirAll(filepath.Join(root, "run-001", "001-step-1"), 0o755); err != nil {
		t.Fatalf("mkdir first: %v", err)
	}
	if got := SequenceScopedPath(root, "run-001", "run-001/step-1"); got != "run-001/001-step-1" {
		t.Fatalf("existing path = %q, want stable path", got)
	}
	if got := SequenceScopedPath(root, "run-001", "run-001/follow-up-graph-planning-001-list_workspace"); got != "run-001/002-follow-up-graph-planning-001-list_workspace" {
		t.Fatalf("follow-up path = %q, want second sequence", got)
	}
	if err := os.MkdirAll(filepath.Join(root, "run-001", "002-follow-up-graph-planning-001-list_workspace"), 0o755); err != nil {
		t.Fatalf("mkdir second: %v", err)
	}
	if got := SequenceScopedPath(root, "run-001", "run-001/repair-001-of-graph-004-004-verify"); got != "run-001/003-repair-001-of-graph-004-004-verify" {
		t.Fatalf("repair path = %q, want third sequence", got)
	}
}

func TestSequenceFromDirName(t *testing.T) {
	if got, ok := SequenceFromDirName("012-repair"); !ok || got != 12 {
		t.Fatalf("SequenceFromDirName = %d,%v want 12,true", got, ok)
	}
	if _, ok := SequenceFromDirName("repair-012"); ok {
		t.Fatal("SequenceFromDirName accepted non-sequence-prefixed name")
	}
}
