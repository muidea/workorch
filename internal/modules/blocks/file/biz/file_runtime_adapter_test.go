package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterProbeRejectsDirectoryPatchTargetWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	patchType, content := "file_write", "new content\n"
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{"."}, 4)
	if err != nil {
		t.Fatal(err)
	}
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "patch-directory", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"."},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "targetRef", String: stringPointer(".")},
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure == nil || failure.Code != "operation_target_not_regular" {
		t.Fatalf("directory patch feasibility failure = %+v", failure)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("probe mutated workspace entries=%+v err=%v", entries, err)
	}
}

func TestRuntimeAdapterAcquireRuntimeTargetSelectionIsCardinalityBounded(t *testing.T) {
	adapter := NewRuntimeAdapter()
	t.Run("unique", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.WriteFile(filepath.Join(workspace, "deliverable.txt"), []byte("value\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-1", "deliverable", runtimeFileTargetSelectorUniqueWorkspaceEntry, 4)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionSelected || len(selection.TargetRefs) != 1 || selection.TargetRefs[0] != "deliverable.txt" || selection.SnapshotID == "" {
			t.Fatalf("unique selection = %+v", selection)
		}
		if selection.SelectorRef != runtimeFileTargetSelectorUniqueWorkspaceEntry {
			t.Fatalf("unique selection lost owner selector binding: %+v", selection)
		}
	})
	t.Run("ambiguous", func(t *testing.T) {
		workspace := t.TempDir()
		for _, name := range []string{"validation-entry.txt", "implementation.txt"} {
			if err := os.WriteFile(filepath.Join(workspace, name), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-2", "deliverable", runtimeFileTargetSelectorUniqueWorkspaceEntry, 1)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionAmbiguous || len(selection.TargetRefs) != 0 || len(selection.CandidateTargetRefs) != 1 || selection.CandidateCount != 2 {
			t.Fatalf("ambiguous selection = %+v", selection)
		}
	})
	t.Run("unknown selector rejected", func(t *testing.T) {
		if _, err := adapter.AcquireRuntimeTargetSelection(t.TempDir(), "target-selection-3", "deliverable", "profile/free_form_selector", 4); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("unknown File selector error = %v, want invalid argument", err)
		}
	})
}

func TestRuntimeAdapterDeclaredNewFileSnapshotAndExecutionGuard(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	declaration := runtimecontract.RuntimeTargetDeclaration{TargetRef: "main.go", TargetRole: "deliverable", Mode: runtimecontract.RuntimeTargetDeclarationNewFile}.Normalized()
	snapshot, err := adapter.AcquireDeclaredTargetSnapshot(ctx, workspace, "main.go", 1024, &declaration)
	if err != nil {
		t.Fatal(err)
	}
	if !runtimecontract.RuntimeTargetSnapshotSatisfiesDeclaration(snapshot, &declaration) || snapshot.Revision == "" || snapshot.WorkspaceTreeRevision == "" {
		t.Fatalf("declared new snapshot = %+v", snapshot)
	}
	patchType, content := patchTypeFileWrite, "package main\n"
	operation := runtimecontract.RuntimeOperation{
		ID: "create", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"main.go"},
		Parameters: []runtimecontract.OperationParameter{{Name: "targetRef", String: stringPointer("main.go")}, {Name: "patchType", String: &patchType}, {Name: "content", String: &content}},
	}
	if failure := adapter.ProbeDeclaredNewFileOperation(workspace, &declaration, operation); failure != nil {
		t.Fatalf("declared new file feasibility = %+v", failure)
	}
	if replay, err := adapter.VerifyDeclaredNewFileWrite(ctx, workspace, &declaration, "main.go", patchType, "", snapshot.Revision, content); err != nil || replay != nil {
		t.Fatalf("new file execution precondition replay=%+v err=%v", replay, err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	complete, err := adapter.AcquireDeclaredTargetSnapshot(ctx, workspace, "main.go", 1024, &declaration)
	if err != nil || !complete.Complete() {
		t.Fatalf("post-create complete snapshot = %+v err=%v", complete, err)
	}
	if replay, err := adapter.VerifyDeclaredNewFileWrite(ctx, workspace, &declaration, "main.go", patchType, "", snapshot.Revision, content); err != nil || replay == nil || replay.AfterHash == "" {
		t.Fatalf("lost-receipt replay replay=%+v err=%v", replay, err)
	}
	if failure := adapter.ProbeDeclaredNewFileOperation(workspace, &declaration, operation); failure == nil || failure.Code != "declared_new_file_already_exists" {
		t.Fatalf("existing declared-new target feasibility = %+v", failure)
	}
}

func TestFileMutationReceiptObservationReportsAfterRevisionForAllStateChangingHandlers(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "target.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// File edit, file write and patch apply all call this owner-local receipt
	// constructor. Keep the variants explicit here so a future handler cannot
	// bypass the required owner-observed target/tree revisions.
	for _, variant := range []string{"file_edit", "file_write", "patch_apply"} {
		t.Run(variant, func(t *testing.T) {
			observation := fileMutationReceiptObservation(workspace, []string{"target.txt"}, runtimecontract.RuntimeOperationExecutionStatusAccepted, RuntimeFileOperationResult{
				BeforeHash: "before-" + variant, AfterHash: "after-" + variant,
			}, "workspace-tree-before-"+variant)
			if observation.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable || !observation.Changed || observation.NoOp {
				t.Fatalf("%s receipt observation state = %+v", variant, observation)
			}
			var target, tree *runtimecontract.RuntimeResourceChange
			for idx := range observation.ResourceChanges {
				change := observation.ResourceChanges[idx]
				switch {
				case change.Domain == runtimecontract.RuntimeResourceWorkspaceTarget && change.SubjectRef == "target.txt":
					target = &change
				case change.Domain == runtimecontract.RuntimeResourceWorkspaceTree && change.SubjectRef == "workspace_deliverable":
					tree = &change
				}
			}
			if target == nil || target.OwnerModuleID != ModuleID || target.BeforeRevision != "before-"+variant || target.AfterRevision != "after-"+variant {
				t.Fatalf("%s target resource change = %+v", variant, observation.ResourceChanges)
			}
			if tree == nil || tree.OwnerModuleID != ModuleID || tree.BeforeRevision != "workspace-tree-before-"+variant || tree.AfterRevision == "" {
				t.Fatalf("%s workspace tree resource change = %+v", variant, observation.ResourceChanges)
			}
		})
	}
	for _, testCase := range []struct {
		name   string
		status runtimecontract.RuntimeOperationExecutionStatus
		result RuntimeFileOperationResult
	}{
		{name: "accepted_no_op", status: runtimecontract.RuntimeOperationExecutionStatusAccepted, result: RuntimeFileOperationResult{BeforeHash: "same", AfterHash: "same"}},
		{name: "rejected", status: runtimecontract.RuntimeOperationExecutionStatusRejected, result: RuntimeFileOperationResult{BeforeHash: "before", AfterHash: "after"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			observation := fileMutationReceiptObservation(workspace, []string{"target.txt"}, testCase.status, testCase.result)
			if len(observation.ResourceChanges) != 0 || observation.Changed {
				t.Fatalf("%s fabricated durable resource changes: %+v", testCase.name, observation)
			}
		})
	}
}

func TestRuntimeAdapterProbeRejectsStaleAdmissibilitySnapshotWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.StepKindFileEdit}, []string{"main.go"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package main\n// changed after inventory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target, oldText, newText := "main.go", "package main\n", "package app\n"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "stale-edit", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"main.go"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "path", String: &target},
			{Name: "oldText", String: &oldText},
			{Name: "newText", String: &newText},
		},
	})
	if failure == nil || failure.Code != "operation_admissibility_stale" {
		t.Fatalf("stale inventory feasibility failure = %+v", failure)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "package main\n// changed after inventory\n" {
		t.Fatalf("probe mutated workspace content=%q err=%v", content, err)
	}
}

func TestRuntimeAdapterAdmissibilityInventoryPrioritizesFocusTargetsWithinBound(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "focus.txt"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := NewRuntimeAdapter().AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.StepKindFileEdit}, []string{"focus.txt"}, 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.TargetRefs) != 2 || inventory.TargetRefs[0] != "focus.txt" || inventory.TargetRefs[1] != "a.txt" {
		t.Fatalf("bounded inventory = %#v, want focus target first then canonical manifest entry", inventory.TargetRefs)
	}
}

func TestRuntimeAdapterProbeAllowsSelectedNewFileWriteTargetWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.TargetRefs) != 0 || len(inventory.SelectorRefs) != 1 || inventory.SelectorRefs[0] != runtimeFileSelectorWorkspaceRelativeNewTarget {
		t.Fatalf("empty-workspace inventory = %+v, want bounded new-target selector", inventory)
	}
	patchType, content, target := "file_write", "package main\n", "main.go"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "create-main", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "targetRef", String: &target},
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure != nil {
		t.Fatalf("new file_write target rejected: %+v", failure)
	}
	if _, err := os.Stat(filepath.Join(workspace, target)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("feasibility probe created target: %v", err)
	}
}

func TestRuntimeAdapterProbeReportsTypedStaleAfterExternalWorkspaceMutation(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "main.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.StepKindFileEdit}, []string{"main.go"}, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	observation := fileAdmissibilityObservation("admissibility-file", inventory, []string{"main.go"}, nil)
	if err := os.WriteFile(target, []byte("package main\n// externally changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := "main.go"
	failure, stale := adapter.ProbeRuntimeOperationFeasibilityWithObservation(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "edit-main", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{path},
		Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &path}},
	}, &observation)
	if failure != nil || stale == nil {
		t.Fatalf("external mutation probe failure/stale=%+v/%+v, want typed stale", failure, stale)
	}
	if stale.PrerequisiteID != observation.PrerequisiteID || stale.ObservationID != observation.ObservationID ||
		stale.Domain != runtimecontract.RuntimeResourceWorkspaceTree || stale.OwnerModuleID != ModuleID ||
		stale.ExpectedRevision != inventory.ObservedRevision || stale.ObservedRevision == inventory.ObservedRevision {
		t.Fatalf("stale = %+v, want exact file-owned observation drift", stale)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "package main\n// externally changed\n" {
		t.Fatalf("probe mutated externally changed target content=%q err=%v", content, err)
	}
}

func TestRuntimeAdapterProbeRejectsNewTargetWithoutOwnerSelector(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	inventory.SelectorRefs = nil
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	patchType, content, target := "file_write", "package main\n", "main.go"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "create-main", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "targetRef", String: &target},
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure == nil || failure.Code != "operation_target_not_admissible" {
		t.Fatalf("new target without selector failure = %+v", failure)
	}
}

func TestRuntimeAdapterProbeNewTargetSelectorRejectsWorkspaceEscape(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	patchType, content, target := "file_write", "outside\n", "../outside.txt"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "escape-workspace", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "targetRef", String: &target},
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure == nil || failure.Code != "operation_target_invalid" {
		t.Fatalf("workspace escape failure = %+v", failure)
	}
}

func TestRuntimeAdapterProbeNewTargetSelectorDoesNotAuthorizeExistingUnlistedTarget(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	patchType, content, target := "file_write", "changed\n", "b.txt"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "overwrite-unlisted", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "targetRef", String: &target},
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure == nil || failure.Code != "operation_target_not_admissible" {
		t.Fatalf("existing unlisted target failure = %+v", failure)
	}
	current, err := os.ReadFile(filepath.Join(workspace, target))
	if err != nil || string(current) != "b.txt\n" {
		t.Fatalf("probe mutated existing target content=%q err=%v", current, err)
	}
}

func TestRuntimeAdapterProbeRejectsMissingTextReplaceDespiteNewTargetSelector(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	patchType, oldText, newText, target := "text_replace", "old", "new", "main.go"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "edit-missing", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "targetRef", String: &target},
			{Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldText},
			{Name: "newText", String: &newText},
		},
	})
	if failure == nil || failure.Code != "operation_target_missing" {
		t.Fatalf("missing text_replace target failure = %+v", failure)
	}
}

func stringPointer(value string) *string { return &value }

func TestRuntimeAdapterReadReturnsContentAndHash(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "main.go",
		StartLine:          1,
		EndLine:            1,
	})
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if result.Content != "package main\n" {
		t.Fatalf("content = %q, want first line", result.Content)
	}
	if result.ContentHash == "" || result.Binary || result.TargetRef != "main.go" ||
		result.ContentComplete || result.SafeForPatch || result.SafeForFileWrite {
		t.Fatalf("read metadata = %+v, want partial read marked unsafe for exact patch", result)
	}
}

func TestRuntimeAdapterReadMarksCompleteFullTextSafeForPatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "main.go",
	})
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !result.ContentComplete || !result.SafeForPatch || !result.SafeForFileWrite {
		t.Fatalf("read metadata = %+v, want complete safe target snapshot metadata", result)
	}
}

func TestRuntimeAdapterReadMarksRedactedAndTruncatedContentUnsafeForPatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "secret.txt"), []byte("token=[REDACTED]\n"), 0o644); err != nil {
		t.Fatalf("seed secret file: %v", err)
	}
	redacted, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "secret.txt",
	})
	if err != nil {
		t.Fatalf("read redacted file: %v", err)
	}
	if !redacted.Redacted || redacted.ContentComplete || redacted.SafeForPatch || redacted.SafeForFileWrite {
		t.Fatalf("redacted output = %+v, want unsafe target snapshot", redacted)
	}

	if err := os.WriteFile(filepath.Join(workspace, "large.txt"), []byte(strings.Repeat("x", defaultMaxReadBytes+1)), 0o644); err != nil {
		t.Fatalf("seed large file: %v", err)
	}
	truncated, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "large.txt",
	})
	if err != nil {
		t.Fatalf("read large file: %v", err)
	}
	if !truncated.Truncated || truncated.ContentComplete || truncated.SafeForPatch || truncated.SafeForFileWrite {
		t.Fatalf("truncated output = %+v, want unsafe target snapshot", truncated)
	}
}

func TestRuntimeAdapterOptionalReadMissingFileCompletes(t *testing.T) {
	workspace := t.TempDir()

	result, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "README.md",
		Optional:           true,
	})
	if err != nil {
		t.Fatalf("optional read missing file: %v", err)
	}
	if result.Status != "completed" || result.Exists || result.Path != "README.md" {
		t.Fatalf("output = %+v, want completed exists=false", result)
	}
}

func TestRuntimeAdapterListSkipsGeneratedDirectories(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "node_modules", "ignored.js"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("seed ignored file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("seed go.mod: %v", err)
	}

	result, err := NewRuntimeAdapter().ListFiles(FileListRequest{
		WorkspaceDirectory: workspace,
		Path:               ".",
		Recursive:          true,
	})
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	seenGoMod := false
	for _, entry := range result.Entries {
		if entry.Path == "node_modules/ignored.js" {
			t.Fatalf("list included skipped generated file: %+v", result.Entries)
		}
		if entry.Path == "go.mod" {
			seenGoMod = true
		}
	}
	if !seenGoMod {
		t.Fatalf("list entries = %+v, want go.mod", result.Entries)
	}
}

func TestRuntimeAdapterListNonRecursiveSkipsIgnoredDirectoriesWithoutFailing(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("seed .git file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}

	result, err := NewRuntimeAdapter().ListFiles(FileListRequest{
		WorkspaceDirectory: workspace,
		Path:               ".",
	})
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].Path != "main.go" {
		t.Fatalf("entries = %+v, want only main.go", result.Entries)
	}
}

func TestAcquireWorkspaceManifestRevisionTracksContent(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "go.mod")
	if err := os.WriteFile(path, []byte("module example.com/one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.TargetRefs) != 1 || first.TargetRefs[0] != "go.mod" || first.Revision == "" {
		t.Fatalf("first manifest = %+v", first)
	}
	if err := os.WriteFile(path, []byte("module example.com/two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision == first.Revision {
		t.Fatalf("manifest revision did not change with content: first=%+v second=%+v", first, second)
	}
}

func TestRuntimeAdapterRejectsPathEscape(t *testing.T) {
	workspace := t.TempDir()
	_, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "../outside.txt",
	})
	if err == nil {
		t.Fatalf("file/read should reject parent traversal")
	}
}

func TestRuntimeAdapterRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "outside")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "outside/secret.txt",
	})
	if err == nil {
		t.Fatalf("file/read should reject symlink escape")
	}
}

func TestRuntimeAdapterTextReplacePreviewDoesNotWrite(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.go")
	original := "package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	sum := sha256.Sum256([]byte(original))
	baseHash := hex.EncodeToString(sum[:])

	result, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.go",
		OldText: "println(\"old\")", NewText: "println(\"new\")", NewTextPresent: true, BaseHash: baseHash,
	})
	if err != nil {
		t.Fatalf("edit file: %v", err)
	}
	if result.Stage != "patch_preview" || result.MatchCount != 1 || result.PatchType != "text_replace" || result.TargetRef != "main.go" {
		t.Fatalf("preview = %+v, want text replacement preview with one match", result)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after edit: %v", err)
	}
	if string(current) != original {
		t.Fatalf("file/edit wrote file; content = %q", string(current))
	}
}

func TestRuntimeAdapterEditRejectsAmbiguousMatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("x\nx\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	_, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.go",
		OldText: "x", NewText: "y", NewTextPresent: true,
	})
	if err == nil {
		t.Fatalf("file/edit should reject ambiguous oldText")
	}
}

func TestRuntimeAdapterEditRejectsBaseHashMismatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	_, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.go",
		OldText: "old", NewText: "new", NewTextPresent: true, BaseHash: "stale",
	})
	if err == nil {
		t.Fatalf("file/edit should reject stale baseHash")
	}
}

func TestRuntimeAdapterTextReplacePreviewUsesExplicitOccurrenceWithoutWriting(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.go")
	original := "x\nx\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.go",
		OldText: "x", NewText: "y", NewTextPresent: true, Occurrence: 2,
	})
	if err != nil {
		t.Fatalf("edit file occurrence: %v", err)
	}
	if result.MatchCount != 2 {
		t.Fatalf("preview = %+v, want two source matches with explicit occurrence", result)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after edit: %v", err)
	}
	if string(current) != original {
		t.Fatalf("file/edit wrote file; content = %q", string(current))
	}
}

func TestRuntimeAdapterEditRejectsBinaryFile(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "asset.bin")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatalf("seed binary file: %v", err)
	}

	_, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "asset.bin",
		OldText: "\x00", NewText: "x", NewTextPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("file/edit binary error = %v, want invalid argument", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read binary after edit: %v", readErr)
	}
	if string(content) != string([]byte{0x00, 0x01, 0x02, 0x03}) {
		t.Fatalf("binary content changed: %v", content)
	}
}

func TestRuntimeAdapterPatchPreviewFileWriteDoesNotWrite(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "README.md")

	result, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "README.md",
		Content: "hello\n", ContentPresent: true,
	})
	if err != nil {
		t.Fatalf("patch preview file_write: %v", err)
	}
	if result.Stage != "patch_preview" || result.WillWrite {
		t.Fatalf("preview output = %+v, want non-writing preview", result)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("patch/preview wrote target or unexpected stat error: %v", err)
	}
}

func TestRuntimeAdapterPatchApplyFileWriteWritesOnce(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "README.md")

	result, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "README.md",
		Content: "hello\n", ContentPresent: true,
	})
	if err != nil {
		t.Fatalf("patch apply file_write: %v", err)
	}
	if result.Stage != "patch_apply" || result.AfterHash == "" || result.ContentHash == "" {
		t.Fatalf("apply output = %+v, want hash metadata", result)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read applied file: %v", err)
	}
	if string(content) != "hello\n" {
		t.Fatalf("applied content = %q", string(content))
	}
}

func TestRuntimeAdapterPatchApplyFileWriteConfirmsSealedReplayAfterReceiptLoss(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "README.md")
	before := []byte("before\n")
	if err := os.WriteFile(target, before, 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	baseHash := sha256Hex(before)
	request := PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: patchTypeFileWrite, TargetRef: "README.md",
		Content: "after\n", ContentPresent: true, BaseHash: baseHash,
	}
	first, err := NewRuntimeAdapter().ApplyPatch(request)
	if err != nil || first.BeforeHash != baseHash || first.AfterHash == "" || first.BeforeHash == first.AfterHash {
		t.Fatalf("initial sealed write = %+v err=%v", first, err)
	}
	// Model a process loss after the durable write and before its owner receipt
	// reached the Orchestrator. The retry sees the sealed postcondition, does
	// not need the original in-memory result, and preserves the same logical
	// resource delta for the persisted execution key/fence.
	replayed, err := NewRuntimeAdapter().ApplyPatch(request)
	if err != nil || replayed.Summary != "patch file_write replay confirmed" || replayed.BeforeHash != baseHash || replayed.AfterHash != first.AfterHash || replayed.ContentHash != first.ContentHash {
		t.Fatalf("lost-receipt replay = %+v err=%v", replayed, err)
	}
	observation := fileMutationReceiptObservation(workspace, replayed.TargetRefs, runtimecontract.RuntimeOperationExecutionStatusAccepted, replayed)
	if !observation.Changed || observation.NoOp || observation.BeforeHash != baseHash || observation.AfterHash != first.AfterHash {
		t.Fatalf("replayed receipt observation lost durable delta: %+v", observation)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "after\n" {
		t.Fatalf("replay changed sealed postcondition: content=%q err=%v", content, err)
	}
}

func TestRuntimeAdapterPatchApplyReplayProofCoversAllDurableVariants(t *testing.T) {
	tests := []struct {
		name       string
		parameters func() []runtimecontract.OperationParameter
	}{
		{
			name: "file_write",
			parameters: func() []runtimecontract.OperationParameter {
				patchType, content, target := patchTypeFileWrite, "after\n", "README.md"
				return []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "content", String: &content}}
			},
		},
		{
			name: "text_replace",
			parameters: func() []runtimecontract.OperationParameter {
				patchType, oldText, newText, target := patchTypeTextReplace, "before", "after", "README.md"
				return []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}
			},
		},
		{
			name: "unified_diff",
			parameters: func() []runtimecontract.OperationParameter {
				patchType, target := patchTypeUnifiedDiff, "README.md"
				diff := "--- a/README.md\n+++ b/README.md\n@@ -1,1 +1,1 @@\n-before\n+after\n"
				return []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("before\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			adapter := NewRuntimeAdapter()
			inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{"README.md"}, 4)
			if err != nil {
				t.Fatal(err)
			}
			operation := runtimecontract.RuntimeOperation{ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"README.md"}, Parameters: test.parameters()}
			failure, stale, proof := adapter.ProbeRuntimeOperationFeasibilityWithReplayProof(workspace, inventory, operation, nil)
			if failure != nil || stale != nil || proof == nil || proof.OwnerModuleID != ModuleID || proof.OperationID != operation.ID || proof.BeforeHash == proof.AfterHash || proof.AdmissibilitySnapshotID != inventory.SnapshotID {
				t.Fatalf("owner replay proof = %+v failure=%+v stale=%+v", proof, failure, stale)
			}
			request, ok := patchApplyRequestFromRuntimeOperation(workspace, operation)
			if !ok {
				t.Fatal("patch request conversion failed")
			}
			request.ReplayProof = proof
			first, err := adapter.ApplyPatch(request)
			if err != nil || first.BeforeHash != proof.BeforeHash || first.AfterHash != proof.AfterHash {
				t.Fatalf("initial proof-bound apply = %+v err=%v", first, err)
			}
			replayed, err := adapter.ApplyPatch(request)
			if err != nil || replayed.Summary != "patch replay proof confirmed" || replayed.BeforeHash != proof.BeforeHash || replayed.AfterHash != proof.AfterHash {
				t.Fatalf("proof replay = %+v err=%v", replayed, err)
			}
			content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
			if err != nil || sha256Hex(content) != proof.AfterHash {
				t.Fatalf("replay changed target content=%q hash=%q err=%v", content, sha256Hex(content), err)
			}
			// A receipt-loss replay may confirm only the postcondition sealed by
			// the original owner feasibility probe. If another writer changes the
			// target before recovery, the File owner must not apply the old patch
			// again or treat the external content as the original operation's
			// durable effect.
			if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("external-change\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.ApplyPatch(request); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("stale replay proof error = %v, want conflict", err)
			}
			content, err = os.ReadFile(filepath.Join(workspace, "README.md"))
			if err != nil || string(content) != "external-change\n" {
				t.Fatalf("stale replay rewrote external content=%q err=%v", content, err)
			}
		})
	}
}

func TestFileOwnerPatchApplyReplaysSealedWriteAfterReceiptCommitFailure(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	workspace := t.TempDir()
	before := []byte("before\n")
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), before, 0o644); err != nil {
		t.Fatal(err)
	}
	baseHash := sha256Hex(before)
	proof := runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: ModuleID, OperationID: "write-1", TargetRefs: []string{"README.md"},
		BeforeHash: baseHash, AfterHash: sha256Hex([]byte("after\n")), AdmissibilitySnapshotID: "admissibility-1",
	}
	commitCalls := 0
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: false}, nil)
	})
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		commitCalls++
		if commitCalls == 1 {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "injected receipt CAS loss", Cause: errors.New("injected receipt CAS loss")})
			return
		}
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})

	command := orchestratorevents.ExecutePatchApplyOperationCommand{
		Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/dispatch-1/write-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, ReplayProof: &proof},
		WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
		OperationID: "write-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"README.md"},
		EvidenceKind: runtimecontract.EvidenceKindStateChange, WorkspaceDeliverableDirectory: workspace,
		Data: orchestratorevents.PatchApplyOperationData{TargetRef: "README.md", PatchType: patchTypeFileWrite, Content: "after\n", ContentPresent: true, BaseHash: baseHash},
	}
	send := func() magicevent.Result {
		return hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecutePatchApplyOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
	}
	if _, err := send().Get(); err == nil {
		t.Fatal("first owner execution unexpectedly succeeded despite receipt commit loss")
	}
	content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "after\n" {
		t.Fatalf("first execution did not apply sealed write: content=%q err=%v", content, err)
	}
	data, resultErr := send().Get()
	if resultErr != nil {
		t.Fatalf("replay after receipt loss: %v", resultErr)
	}
	response, ok := data.(orchestratorevents.ExecutePatchApplyOperationResult)
	if !ok || response.Receipt.ExecutionKey != command.Execution.ExecutionKey || !response.Receipt.Changed || response.Receipt.BeforeHash != baseHash || response.Receipt.AfterHash == baseHash || commitCalls != 2 {
		t.Fatalf("owner replay did not commit the sealed logical delta once: response=%+v calls=%d", response, commitCalls)
	}
	content, err = os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "after\n" {
		t.Fatalf("replay rewrote sealed postcondition: content=%q err=%v", content, err)
	}
}

func TestRuntimeAdapterProbeRejectsUnverifiedFileWriteReplayBaseHash(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("current\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{"README.md"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	patchType, content, staleHash := patchTypeFileWrite, "after\n", "unverified-prior-revision"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "sealed-file-write", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"README.md"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "targetRef", String: stringPointer("README.md")}, {Name: "patchType", String: &patchType},
			{Name: "content", String: &content}, {Name: "baseHash", String: &staleHash},
		},
	})
	if failure == nil || failure.Code != "operation_base_hash_mismatch" {
		t.Fatalf("unverified replay precondition = %+v", failure)
	}
}

func TestRuntimeAdapterPatchApplyRejectsBaseHashMismatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("current\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "README.md",
		Content: "next\n", ContentPresent: true, BaseHash: "stale",
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("patch/apply baseHash error = %v, want conflict", err)
	}
	content, readErr := os.ReadFile(filepath.Join(workspace, "README.md"))
	if readErr != nil {
		t.Fatalf("read after rejected patch: %v", readErr)
	}
	if string(content) != "current\n" {
		t.Fatalf("rejected patch changed file: %q", string(content))
	}
}

func TestRuntimeAdapterPatchApplyTextReplaceRejectsMissingOldText(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.txt",
		OldText: "missing", NewText: "new", NewTextPresent: true,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("patch/apply oldText error = %v, want conflict", err)
	}
}

func TestRuntimeAdapterPatchApplyTextReplaceRejectsAmbiguousOldText(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("x\nx\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.txt",
		OldText: "x", NewText: "y", NewTextPresent: true,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("patch/apply ambiguous oldText error = %v, want conflict", err)
	}
}

func TestRuntimeAdapterPatchApplyRejectsBinaryTarget(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "asset.bin")
	if err := os.WriteFile(path, []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatalf("seed binary file: %v", err)
	}

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "asset.bin",
		Content: "text\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("patch/apply binary target error = %v, want invalid argument", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read binary file: %v", readErr)
	}
	if string(content) != string([]byte{0, 1, 2}) {
		t.Fatalf("binary target changed: %v", content)
	}
}

func TestRuntimeAdapterPatchApplyRejectsPathEscape(t *testing.T) {
	workspace := t.TempDir()
	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "../outside.txt",
		Content: "outside\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("patch/apply path escape error = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterPatchApplyRejectsAbsolutePath(t *testing.T) {
	workspace := t.TempDir()
	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: filepath.Join(workspace, "outside.txt"),
		Content: "outside\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("patch/apply absolute path error = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterPatchApplyRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "outside")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "outside/file.txt",
		Content: "outside\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("patch/apply symlink escape error = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterPatchApplyRejectsMissingParentConflict(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "dir"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("seed parent file: %v", err)
	}
	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "dir/file.txt",
		Content: "hello\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("patch/apply parent conflict error = %v, want conflict", err)
	}
}

func TestRuntimeAdapterPatchApplyConsumesValidatedApprovalGrant(t *testing.T) {
	workspace := t.TempDir()
	request := PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: ".env",
		Content: "TOKEN=granted\n", ContentPresent: true,
	}
	if _, err := NewRuntimeAdapter().ApplyPatch(request); err == nil {
		t.Fatal("sensitive patch/apply must require approval before side effects")
	}
	if _, err := os.Stat(filepath.Join(workspace, ".env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("approval-required preview changed file: %v", err)
	}
	request.ApprovalGranted = true
	result, err := NewRuntimeAdapter().ApplyPatch(request)
	if err != nil {
		t.Fatalf("granted patch/apply: %v", err)
	}
	content, readErr := os.ReadFile(filepath.Join(workspace, ".env"))
	if readErr != nil || string(content) != "TOKEN=granted\n" || result.AfterHash == "" {
		t.Fatalf("granted patch result=%+v content=%q err=%v", result, string(content), readErr)
	}
}

func TestRuntimeAdapterPatchApplyUnifiedDiffSuccess(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	diff := "--- a/main.txt\n+++ b/main.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"

	result, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "unified_diff", TargetRef: "main.txt", Diff: diff,
	})
	if err != nil {
		t.Fatalf("patch/apply unified diff: %v", err)
	}
	if result.DiffHash == "" || result.AddedLines != 1 || result.DeletedLines != 1 {
		t.Fatalf("unified diff output = %+v, want diff metadata", result)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read patched file: %v", readErr)
	}
	if string(content) != "one\nTWO\nthree\n" {
		t.Fatalf("patched content = %q", string(content))
	}
}

func TestRuntimeAdapterPatchApplyUnifiedDiffRejectsStaleContext(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("one\nchanged\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	diff := "--- a/main.txt\n+++ b/main.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "unified_diff", TargetRef: "main.txt", Diff: diff,
	})
	if err == nil {
		t.Fatalf("patch/apply unified diff should reject stale context")
	}
	content, readErr := os.ReadFile(filepath.Join(workspace, "main.txt"))
	if readErr != nil {
		t.Fatalf("read after rejected diff: %v", readErr)
	}
	if string(content) != "one\nchanged\nthree\n" {
		t.Fatalf("rejected diff changed file: %q", string(content))
	}
}

func TestRuntimeAdapterRuntimeFeasibilityRejectsStaleUnifiedDiff(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("one\nchanged\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{"main.txt"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	patchType, target, diff := "unified_diff", "main.txt", "--- a/main.txt\n+++ b/main.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "stale-diff", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType}, {Name: "targetRef", String: &target}, {Name: "diff", String: &diff},
		},
	})
	if failure == nil || failure.Code != "unified_diff_apply_conflict" {
		t.Fatalf("failure = %+v, want unified_diff_apply_conflict", failure)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "main.txt"))
	if err != nil || string(content) != "one\nchanged\nthree\n" {
		t.Fatalf("read-only feasibility changed file content=%q err=%v", string(content), err)
	}
}
