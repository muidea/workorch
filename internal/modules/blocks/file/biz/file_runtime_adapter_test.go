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
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterReadReturnsContentAndHash(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/read",
		Input: map[string]any{
			"path":      "main.go",
			"startLine": 1,
			"endLine":   1,
		},
	})
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if result.Output["content"] != "package main\n" {
		t.Fatalf("content = %q, want first line", result.Output["content"])
	}
	if result.Output["contentHash"] == "" ||
		result.Output["isBinary"] != false ||
		result.Output["targetRef"] != "main.go" ||
		result.Output["contentComplete"] != false ||
		result.Output["safeForPatch"] != false ||
		result.Output["safeForFileWrite"] != false {
		t.Fatalf("read metadata = %+v, want partial read marked unsafe for exact patch", result.Output)
	}
}

func TestRuntimeAdapterReadMarksCompleteFullTextSafeForPatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/read",
		Input:                         map[string]any{"path": "main.go"},
	})
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if result.Output["contentComplete"] != true ||
		result.Output["safeForPatch"] != true ||
		result.Output["safeForFileWrite"] != true {
		t.Fatalf("read metadata = %+v, want complete safe target snapshot metadata", result.Output)
	}
}

func TestRuntimeAdapterReadMarksRedactedAndTruncatedContentUnsafeForPatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "secret.txt"), []byte("token=[REDACTED]\n"), 0o644); err != nil {
		t.Fatalf("seed secret file: %v", err)
	}
	redacted, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/read",
		Input:                         map[string]any{"path": "secret.txt"},
	})
	if err != nil {
		t.Fatalf("read redacted file: %v", err)
	}
	if redacted.Output["redacted"] != true ||
		redacted.Output["contentComplete"] != false ||
		redacted.Output["safeForPatch"] != false ||
		redacted.Output["safeForFileWrite"] != false {
		t.Fatalf("redacted output = %+v, want unsafe target snapshot", redacted.Output)
	}

	if err := os.WriteFile(filepath.Join(workspace, "large.txt"), []byte(strings.Repeat("x", defaultMaxReadBytes+1)), 0o644); err != nil {
		t.Fatalf("seed large file: %v", err)
	}
	truncated, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/read",
		Input:                         map[string]any{"path": "large.txt"},
	})
	if err != nil {
		t.Fatalf("read large file: %v", err)
	}
	if truncated.Output["truncated"] != true ||
		truncated.Output["contentComplete"] != false ||
		truncated.Output["safeForPatch"] != false ||
		truncated.Output["safeForFileWrite"] != false {
		t.Fatalf("truncated output = %+v, want unsafe target snapshot", truncated.Output)
	}
}

func TestRuntimeAdapterOptionalReadMissingFileCompletes(t *testing.T) {
	workspace := t.TempDir()

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/read",
		Input: map[string]any{
			"path":     "README.md",
			"optional": true,
		},
	})
	if err != nil {
		t.Fatalf("optional read missing file: %v", err)
	}
	if result.Output["status"] != "completed" || result.Output["exists"] != false || result.Output["path"] != "README.md" {
		t.Fatalf("output = %+v, want completed exists=false", result.Output)
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

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/list",
		Input: map[string]any{
			"path":      ".",
			"recursive": true,
		},
	})
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	entries, ok := result.Output["entries"].([]map[string]any)
	if !ok {
		t.Fatalf("entries = %T, want []map[string]any", result.Output["entries"])
	}
	seenGoMod := false
	for _, entry := range entries {
		if entry["path"] == "node_modules/ignored.js" {
			t.Fatalf("list included skipped generated file: %+v", entries)
		}
		if entry["path"] == "go.mod" {
			seenGoMod = true
		}
	}
	if !seenGoMod {
		t.Fatalf("list entries = %+v, want go.mod", entries)
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

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/list",
		Input:                         map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	entries, ok := result.Output["entries"].([]map[string]any)
	if !ok {
		t.Fatalf("entries = %T, want []map[string]any", result.Output["entries"])
	}
	if len(entries) != 1 || entries[0]["path"] != "main.go" {
		t.Fatalf("entries = %+v, want only main.go", entries)
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
	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/read",
		Input:                         map[string]any{"path": "../outside.txt"},
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

	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/read",
		Input:                         map[string]any{"path": "outside/secret.txt"},
	})
	if err == nil {
		t.Fatalf("file/read should reject symlink escape")
	}
}

func TestRuntimeAdapterEditReturnsPatchPreviewWithoutWriting(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.go")
	original := "package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	sum := sha256.Sum256([]byte(original))
	baseHash := hex.EncodeToString(sum[:])

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/edit",
		Input: map[string]any{
			"path":     "main.go",
			"oldText":  "println(\"old\")",
			"newText":  "println(\"new\")",
			"baseHash": baseHash,
		},
	})
	if err != nil {
		t.Fatalf("edit file: %v", err)
	}
	if result.Output["stage"] != "file_edit_preview" || result.Output["matchCount"] != 1 {
		t.Fatalf("edit output = %+v, want preview with one match", result.Output)
	}
	preview, ok := result.Output["patchPreview"].(map[string]any)
	if !ok {
		t.Fatalf("patchPreview = %T, want map", result.Output["patchPreview"])
	}
	diff, _ := preview["diff"].(string)
	if !strings.Contains(diff, "-\tprintln(\"old\")") || !strings.Contains(diff, "+\tprintln(\"new\")") {
		t.Fatalf("diff = %q, want old/new replacement", diff)
	}
	if preview["patchType"] != "unified_diff" || preview["targetRef"] != "main.go" {
		t.Fatalf("patchPreview = %+v, want unified diff target", preview)
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
	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/edit",
		Input: map[string]any{
			"path":    "main.go",
			"oldText": "x",
			"newText": "y",
		},
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
	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/edit",
		Input: map[string]any{
			"path":     "main.go",
			"oldText":  "old",
			"newText":  "new",
			"baseHash": "stale",
		},
	})
	if err == nil {
		t.Fatalf("file/edit should reject stale baseHash")
	}
}

func TestRuntimeAdapterEditUsesExplicitOccurrenceWithoutWriting(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.go")
	original := "x\nx\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/edit",
		Input: map[string]any{
			"path":       "main.go",
			"oldText":    "x",
			"newText":    "y",
			"occurrence": 2,
		},
	})
	if err != nil {
		t.Fatalf("edit file occurrence: %v", err)
	}
	if result.Output["matchCount"] != 2 || result.Output["selectedOccurrence"] != 2 {
		t.Fatalf("edit output = %+v, want second occurrence selected", result.Output)
	}
	preview, ok := result.Output["patchPreview"].(map[string]any)
	if !ok {
		t.Fatalf("patchPreview = %T, want map", result.Output["patchPreview"])
	}
	diff, _ := preview["diff"].(string)
	if !strings.Contains(diff, "-x\n+y") {
		t.Fatalf("diff = %q, want second occurrence replacement", diff)
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

	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "file/edit",
		Input: map[string]any{
			"path":    "asset.bin",
			"oldText": "\x00",
			"newText": "x",
		},
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

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/preview",
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "README.md",
			"content":   "hello\n",
		},
	})
	if err != nil {
		t.Fatalf("patch preview file_write: %v", err)
	}
	if result.Output["stage"] != "patch_preview" || result.Output["willWrite"] != false {
		t.Fatalf("preview output = %+v, want non-writing preview", result.Output)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("patch/preview wrote target or unexpected stat error: %v", err)
	}
}

func TestRuntimeAdapterPatchApplyFileWriteWritesOnce(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "README.md")

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "README.md",
			"content":   "hello\n",
		},
	})
	if err != nil {
		t.Fatalf("patch apply file_write: %v", err)
	}
	if result.Output["stage"] != "patch_apply" || result.Output["afterHash"] == "" || result.Output["contentHash"] == "" {
		t.Fatalf("apply output = %+v, want hash metadata", result.Output)
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

	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "README.md",
			"content":   "next\n",
			"baseHash":  "stale",
		},
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

	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "text_replace",
			"targetRef": "main.txt",
			"oldText":   "missing",
			"newText":   "new",
		},
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

	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "text_replace",
			"targetRef": "main.txt",
			"oldText":   "x",
			"newText":   "y",
		},
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

	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "asset.bin",
			"content":   "text\n",
		},
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
	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "../outside.txt",
			"content":   "outside\n",
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("patch/apply path escape error = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterPatchApplyRejectsAbsolutePath(t *testing.T) {
	workspace := t.TempDir()
	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": filepath.Join(workspace, "outside.txt"),
			"content":   "outside\n",
		},
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
	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "outside/file.txt",
			"content":   "outside\n",
		},
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
	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "dir/file.txt",
			"content":   "hello\n",
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("patch/apply parent conflict error = %v, want conflict", err)
	}
}

func TestRuntimeAdapterPatchApplyUnifiedDiffSuccess(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	diff := "--- a/main.txt\n+++ b/main.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "unified_diff",
			"targetRef": "main.txt",
			"diff":      diff,
		},
	})
	if err != nil {
		t.Fatalf("patch/apply unified diff: %v", err)
	}
	if result.Output["diffHash"] == "" || result.Output["addedLines"] != 1 || result.Output["deletedLines"] != 1 {
		t.Fatalf("unified diff output = %+v, want diff metadata", result.Output)
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

	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: workspace,
		Kind:                          "patch/apply",
		Input: map[string]any{
			"patchType": "unified_diff",
			"targetRef": "main.txt",
			"diff":      diff,
		},
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
