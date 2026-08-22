package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateLikeNameRequiresExplicitStateVocabularySuffix(t *testing.T) {
	for _, name := range []string{"WorkspaceState", "RuntimeStatus", "planningPhase", "CandidateDisposition", "OperationOutcome", "loadMode", "RunLifecycle", "DriveStage"} {
		if !isStateLikeName(name) {
			t.Fatalf("%q was not classified as state-like", name)
		}
	}
	for _, name := range []string{"EvidenceQuality", "CheckpointType", "StatefulValue"} {
		if isStateLikeName(name) {
			t.Fatalf("%q was incorrectly classified as state-like", name)
		}
	}
}

func TestVerifyRejectsInvariantWithMultipleTypedConstants(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"application/demo", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte(`package demo
type FixedState string
const (
	FixedStateReady FixedState = "ready"
	FixedStateDone FixedState = "done"
)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte("application/demo/state.go|FixedState|invariant_value|application/demo|-\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "exactly 1") {
		t.Fatalf("verify error = %v, want invariant constant-count rejection", err)
	}
}
