package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadArtifactContentWithBackendReturnsStoredArtifactFile(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-artifact-content")
	runID := common.RunID("run-artifact-content")
	artifactURI := "workorch://workspaces/ws-artifact-content/artifacts/run-artifact-content/result-summary.txt"
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	dataRoot := t.TempDir()
	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "artifact content", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("Create run: %v", err)
	}
	artifact := &resultmodel.Artifact{
		ID:           common.ArtifactID("run-artifact-content/artifact/result-summary"),
		WorkspaceID:  workspaceID,
		RunID:        runID,
		ResultID:     common.ResultID("run-artifact-content/result/dod-assessment"),
		ArtifactType: resultcommon.ArtifactTypeText,
		Name:         "Accepted projectioncontract.Result Summary",
		Summary:      "workorchprojectioncontract.Artifact content from repository data dir",
		URI:          artifactURI,
		State:        common.ArtifactStateAvailable,
	}
	if err := exports.Repositories.Artifacts.Create(ctx, artifact); err != nil {
		t.Fatalf("Create artifact: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("Close session: %v", err)
	}

	item, err := ReadArtifactContentWithBackend(context.Background(), "file", dataRoot, artifactURI)
	if err != nil {
		t.Fatalf("ReadArtifactContentWithBackend: %v", err)
	}
	if item.FileName != "result-summary.txt" || item.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("artifact content metadata = %+v, want result-summary text", item)
	}
	if string(item.Content) != "workorchprojectioncontract.Artifact content from repository data dir\n" {
		t.Fatalf("content = %q, want stored artifact summary", string(item.Content))
	}
}

func TestReadArtifactContentWithBackendRejectsUnknownOrUnsafeURI(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataRoot, "internal", "workspaces", "ws-a", "artifacts", "run-a"), 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "internal", "workspaces", "ws-a", "artifacts", "run-a", "secret.txt"), []byte("secret"), 0o600); err != nil {
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
