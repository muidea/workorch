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

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeAdapterReadReturnsContentAndHash(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceWorkingDirectory: workspace,
		Kind:                      "file/read",
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
	if result.Output["contentHash"] == "" || result.Output["isBinary"] != false {
		t.Fatalf("read metadata = %+v, want content hash and text", result.Output)
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
		WorkspaceWorkingDirectory: workspace,
		Kind:                      "file/list",
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

func TestRuntimeAdapterRejectsPathEscape(t *testing.T) {
	workspace := t.TempDir()
	_, err := NewRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceWorkingDirectory: workspace,
		Kind:                      "file/read",
		Input:                     map[string]any{"path": "../outside.txt"},
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
		WorkspaceWorkingDirectory: workspace,
		Kind:                      "file/read",
		Input:                     map[string]any{"path": "outside/secret.txt"},
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
		WorkspaceWorkingDirectory: workspace,
		Kind:                      "file/edit",
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
		WorkspaceWorkingDirectory: workspace,
		Kind:                      "file/edit",
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
		WorkspaceWorkingDirectory: workspace,
		Kind:                      "file/edit",
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
		WorkspaceWorkingDirectory: workspace,
		Kind:                      "file/edit",
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
		WorkspaceWorkingDirectory: workspace,
		Kind:                      "file/edit",
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
