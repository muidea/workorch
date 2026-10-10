package appservice

import (
	"context"
	"errors"
	"testing"

	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadArtifactContentUsesResultOwnerAcrossBackends(t *testing.T) {
	for _, backend := range []string{"memory", "file"} {
		t.Run(backend, func(t *testing.T) {
			workspaceID := common.WorkspaceID("ws-artifact-content")
			runID := common.RunID("run-artifact-content")
			artifactURI := "workorch://workspaces/ws-artifact-content/artifacts/run-artifact-content/result-summary.txt"
			session, ctx := newTestRuntimeSession(t, backend, t.TempDir())
			ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
			exports := testRuntimeExports(session.Bootstrap)
			if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "artifact content", State: common.WorkspaceStateActive}); err != nil {
				t.Fatal(err)
			}
			if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
				t.Fatal(err)
			}
			artifact := &resultmodel.Artifact{
				ID: common.ArtifactID("run-artifact-content/artifact/result-summary"), WorkspaceID: workspaceID, RunID: runID,
				ResultID: common.ResultID("run-artifact-content/result/dod-assessment"), ArtifactType: resultcommon.ArtifactTypeText,
				Name: "Accepted Result Summary", Summary: "Artifact content from Result owner", URI: artifactURI, State: common.ArtifactStateAvailable,
			}
			if err := exports.Repositories.Artifacts.Create(ctx, artifact); err != nil {
				t.Fatal(err)
			}

			item, err := ReadArtifactContentWithSession(ctx, session, artifactURI)
			if err != nil {
				t.Fatal(err)
			}
			if item.FileName != "result-summary.txt" || item.ContentType != "text/plain; charset=utf-8" || len(item.ContentSHA256) != 64 {
				t.Fatalf("artifact content metadata = %+v", item)
			}
			if string(item.Content) != "Artifact content from Result owner\n" {
				t.Fatalf("content = %q", item.Content)
			}
		})
	}
}

func TestReadArtifactContentWithBackendRejectsUnknownOrUnsafeURI(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()

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
