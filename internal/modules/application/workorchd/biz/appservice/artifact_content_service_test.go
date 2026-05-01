package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadArtifactContentWithBackendReturnsStoredArtifactFile(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	artifact := &resultmodel.Artifact{
		ID:           common.ArtifactID("run-artifact-content/artifact/result-summary"),
		WorkspaceID:  common.WorkspaceID("ws-artifact-content"),
		RunID:        common.RunID("run-artifact-content"),
		ResultID:     common.ResultID("run-artifact-content/result/dod-assessment"),
		ArtifactType: resultcommon.ArtifactTypeText,
		Name:         "Accepted Result Summary",
		Summary:      "Artifact content from repository data dir",
		State:        common.ArtifactStateAvailable,
	}
	if err := exports.Repositories.Artifacts.Create(ctx, artifact); err != nil {
		t.Fatalf("Create artifact: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("Close session: %v", err)
	}

	item, err := ReadArtifactContentWithBackend(ctx, "file", dataRoot, "workorch://workspaces/ws-artifact-content/artifacts/run-artifact-content/result-summary.txt")
	if err != nil {
		t.Fatalf("ReadArtifactContentWithBackend: %v", err)
	}
	if item.FileName != "result-summary.txt" || item.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("artifact content metadata = %+v, want result-summary text", item)
	}
	if string(item.Content) != "Artifact content from repository data dir\n" {
		t.Fatalf("content = %q, want stored artifact summary", string(item.Content))
	}
}

func TestReadArtifactContentWithRunSnapshotReusesCachedArtifactMetadata(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspaceID := "ws-artifact-cache"
	runID := "run-artifact-cache"
	artifactURI := "workorch://workspaces/ws-artifact-cache/artifacts/run-artifact-cache/result-summary.txt"
	artifactDir := filepath.Join(dataRoot, "workspaces", workspaceID, "artifacts", runID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "result-summary.txt"), []byte("cached artifact payload"), 0o600); err != nil {
		t.Fatalf("write artifact file: %v", err)
	}
	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	defer func() {
		if err := session.Close(ctx); err != nil {
			t.Fatalf("Close session: %v", err)
		}
	}()

	item, err := ReadArtifactContentWithRunSnapshotWithSession(ctx, session.Borrow(), artifactURI, RunSnapshot{
		Run: Run{ID: runID, WorkspaceID: workspaceID, State: string(common.RunStateCompleted)},
		Artifacts: []Artifact{{
			WorkspaceID: workspaceID,
			RunID:       runID,
			URI:         artifactURI,
			ContentType: "text/markdown; charset=utf-8",
			State:       string(common.ArtifactStateAvailable),
		}},
	})
	if err != nil {
		t.Fatalf("ReadArtifactContentWithRunSnapshotWithSession: %v", err)
	}
	if item.ContentType != "text/markdown; charset=utf-8" {
		t.Fatalf("content type = %q, want cached artifact metadata", item.ContentType)
	}
	if string(item.Content) != "cached artifact payload" {
		t.Fatalf("content = %q, want cached artifact payload", string(item.Content))
	}
}

func TestReadArtifactContentWithBackendRejectsUnknownOrUnsafeURI(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataRoot, "workspaces", "ws-a", "artifacts", "run-a"), 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "workspaces", "ws-a", "artifacts", "run-a", "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	if _, err := ReadArtifactContentWithBackend(ctx, "file", dataRoot, "file:///tmp/secret"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("external uri err = %v, want invalid argument", err)
	}
	if _, err := ReadArtifactContentWithBackend(ctx, "file", dataRoot, "workorch://workspaces/ws-a/artifacts/run-a/../secret.txt"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unsafe uri err = %v, want invalid argument", err)
	}
	if _, err := ReadArtifactContentWithBackend(ctx, "file", dataRoot, "workorch://workspaces/ws-a/artifacts/run-a/secret.txt"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("unregistered uri err = %v, want not found", err)
	}
}
