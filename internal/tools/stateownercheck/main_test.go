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

func TestVerifyRejectsTransitionBearingMachineWithoutNamedStateType(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"application/demo", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte("package demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte("demo.lifecycle|resource_state_machine|application/demo|application/demo/state.go|application/demo/state.go|applyDemoEvent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "without a named state type") {
		t.Fatalf("verify error = %v, want missing named state type rejection", err)
	}
}

func TestVerifyAllowsSameOwnerCoordinatedTransitions(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"application/demo", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte("package demo\ntype RunState string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte("application/demo/state.go|RunState|authoritative_state_machine|application/demo|demo.lifecycle,demo.replacement\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transitions := "demo.lifecycle|resource_state_machine|application/demo|application/demo/lifecycle_internal_event.go|application/demo/lifecycle_internal_event.go|submitLifecycleInternalEvent\n" +
		"demo.replacement|owner_private_transaction|application/demo|application/demo/replacement_internal_event.go|application/demo/replacement_internal_event.go|submitReplacementInternalEvent\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transitions), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest"); err != nil {
		t.Fatalf("same-owner coordinated transitions must be allowed: %v", err)
	}
}

func TestVerifyRejectsCoordinatedTransitionsAcrossOwners(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"application/demo", "internal", "scripts"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "application/demo/state.go"), []byte("package demo\ntype RunState string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/state-model-owners.manifest"), []byte("application/demo/state.go|RunState|authoritative_state_machine|application/demo|demo.lifecycle,demo.foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transitions := "demo.lifecycle|resource_state_machine|application/demo|application/demo/lifecycle_internal_event.go|application/demo/lifecycle_internal_event.go|submitLifecycleInternalEvent\n" +
		"demo.foreign|owner_private_transaction|internal/other|internal/other/foreign_internal_event.go|internal/other/foreign_internal_event.go|submitForeignInternalEvent\n"
	if err := os.WriteFile(filepath.Join(root, "scripts/state-transition-owners.manifest"), []byte(transitions), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verify(root, "scripts/state-model-owners.manifest", "scripts/state-transition-owners.manifest")
	if err == nil || !strings.Contains(err.Error(), "transition demo.foreign is owned by internal/other") {
		t.Fatalf("verify error = %v, want cross-owner coordination rejection", err)
	}
}
