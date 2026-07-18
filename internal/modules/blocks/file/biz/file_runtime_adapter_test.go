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

	"github.com/muidea/workorch/internal/pkg/common"
)

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
