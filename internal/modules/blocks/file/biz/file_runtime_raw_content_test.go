package file

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeFileReadPreservesLiteralMarkersAsCompleteRawContent(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
	}{
		{"source", "package example\nvar replacement = []string{\"[REDACTED]\"}\n"},
		{"template", "  用户模板\r\npassword: [SECRET]\r\n\r\n"},
		{"pattern", "^(?:[MASKED]|[redacted]|[secret]|[masked])$"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			workspace := t.TempDir()
			const target = "input.txt"
			raw := []byte(test.content)
			if err := os.WriteFile(filepath.Join(workspace, target), raw, 0o600); err != nil {
				t.Fatal(err)
			}
			var previous runtimecontract.TargetSnapshot
			for attempt := 0; attempt < 2; attempt++ {
				adapter := NewRuntimeAdapter()
				read, err := adapter.ReadFile(context.Background(), FileReadRequest{WorkspaceDirectory: workspace, Path: target})
				if err != nil {
					t.Fatal(err)
				}
				if read.Content != test.content || read.ContentHash != sha256Hex(raw) || read.Size != int64(len(raw)) ||
					read.Redacted || read.Binary || read.Truncated || !read.ContentComplete || !read.SafeForPatch || !read.SafeForFileWrite {
					t.Fatalf("raw content was changed or misclassified: %+v", read)
				}
				snapshot, err := adapter.AcquireTargetSnapshot(context.Background(), workspace, target, len(raw))
				if err != nil || !snapshot.Complete() || snapshot.Content != test.content || snapshot.ContentHash != read.ContentHash || snapshot.Size != read.Size {
					t.Fatalf("complete raw snapshot lost: %+v, err=%v", snapshot, err)
				}
				if attempt > 0 && !reflect.DeepEqual(previous, snapshot) {
					t.Fatal("fresh adapter changed the same raw snapshot")
				}
				previous = snapshot
			}
			actual, err := os.ReadFile(filepath.Join(workspace, target))
			if err != nil || string(actual) != test.content {
				t.Fatalf("read modified source bytes: %v", err)
			}
		})
	}
}

func TestRuntimeFileMarkerContentRetainsIncompleteReadGuards(t *testing.T) {
	workspace := t.TempDir()
	const content = "[REDACTED]\n[SECRET]\n[MASKED]\n"
	if err := os.WriteFile(filepath.Join(workspace, "input.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	for _, request := range []FileReadRequest{
		{WorkspaceDirectory: workspace, Path: "input.txt", MaxBytes: 5},
		{WorkspaceDirectory: workspace, Path: "input.txt", StartLine: 1, EndLine: 1},
	} {
		read, err := adapter.ReadFile(context.Background(), request)
		if err != nil || read.ContentComplete || read.SafeForPatch || read.SafeForFileWrite || read.Redacted || read.ContentHash != sha256Hex([]byte(content)) {
			t.Fatalf("partial read lost its guard or whole-file hash: %+v, err=%v", read, err)
		}
	}
	if snapshot, err := adapter.AcquireTargetSnapshot(context.Background(), workspace, "input.txt", 5); err == nil || snapshot.ContentState != runtimecontract.SnapshotContentStateTruncated {
		t.Fatalf("truncated marker content accepted: %+v, err=%v", snapshot, err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "binary.txt"), append([]byte(content), 0xff), 0o600); err != nil {
		t.Fatal(err)
	}
	if snapshot, err := adapter.AcquireTargetSnapshot(context.Background(), workspace, "binary.txt", 1024); err == nil || snapshot.ContentState != runtimecontract.SnapshotContentStateBinary {
		t.Fatalf("binary marker content accepted: %+v, err=%v", snapshot, err)
	}
	if _, err := adapter.AcquireTargetSnapshot(context.Background(), workspace, "missing.txt", 1024); err == nil {
		t.Fatal("missing content accepted")
	}
}
