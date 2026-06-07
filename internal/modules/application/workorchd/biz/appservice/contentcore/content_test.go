package contentcore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestParseArtifactURI(t *testing.T) {
	ref, err := ParseArtifactURI("workorch://workspaces/ws-001/artifacts/run-001/result-summary.txt")
	if err != nil {
		t.Fatalf("ParseArtifactURI err = %v", err)
	}
	if ref.WorkspaceID != "ws-001" || ref.RunID != "run-001" || ref.RelativePath != "result-summary.txt" {
		t.Fatalf("artifact ref = %+v", ref)
	}
	if _, err := ParseArtifactURI("workorch://workspaces/ws-001/artifacts/run-001/../secret.txt"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unsafe artifact uri err = %v, want invalid argument", err)
	}
}

func TestSafeArtifactPathRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	path, err := SafeArtifactPath(root, "nested/result.txt")
	if err != nil {
		t.Fatalf("SafeArtifactPath err = %v", err)
	}
	if filepath.Dir(path) != filepath.Join(root, "nested") {
		t.Fatalf("safe artifact path = %s", path)
	}
	if _, err := SafeArtifactPath(root, "../secret.txt"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unsafe artifact path err = %v, want invalid argument", err)
	}
}

func TestParseRuntimeRawRefAndContentType(t *testing.T) {
	ref, err := ParseRuntimeRawRef("workorch://workspaces/ws-001/runs/run-001/llm-calls/run-001/llm-1/response.sse")
	if err != nil {
		t.Fatalf("ParseRuntimeRawRef err = %v", err)
	}
	if ref.WorkspaceID != "ws-001" || ref.RunID != "run-001" || ref.Collection != "llm-calls" || ref.ItemPath != "run-001/llm-1" || ref.FileName != "response.sse" {
		t.Fatalf("runtime raw ref = %+v", ref)
	}
	if RuntimeRawContentType(ref.FileName) != "text/event-stream; charset=utf-8" {
		t.Fatalf("content type = %q", RuntimeRawContentType(ref.FileName))
	}
	if _, err := ParseRuntimeRawRef("workorch://workspaces/ws-001/runs/run-001/llm-calls/run-001/llm-1/secret.txt"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid runtime raw ref err = %v, want invalid argument", err)
	}
}

func TestRuntimeRawItemRootFindsSequencedPath(t *testing.T) {
	collectionRoot := t.TempDir()
	runID := common.RunID("run-001")
	sequenced := filepath.Join(collectionRoot, string(runID), "001-step-1")
	if err := os.MkdirAll(sequenced, 0o755); err != nil {
		t.Fatalf("mkdir sequenced path: %v", err)
	}
	root, err := RuntimeRawItemRoot(collectionRoot, runID, "run-001/step-1")
	if err != nil {
		t.Fatalf("RuntimeRawItemRoot err = %v", err)
	}
	if root != sequenced {
		t.Fatalf("RuntimeRawItemRoot = %s, want %s", root, sequenced)
	}
}
