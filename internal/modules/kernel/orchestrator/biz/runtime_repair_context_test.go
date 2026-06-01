package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRepairSubsystemRuntimeRepairPatchContextReadsCurrentWorkspaceTarget(t *testing.T) {
	ctx := context.Background()
	deliverableDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDir, "source.txt"), []byte("current content\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	workspaceRepo := memory.NewWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-repair-context")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "repair context",
		State:                common.WorkspaceStateActive,
		DeliverableDirectory: deliverableDir,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{ID: "run-repair-context", WorkspaceID: workspaceID}
	failedStep := &runmodel.Step{
		ID:    "run-repair-context/patch-preview",
		RunID: run.ID,
		Kind:  "patch/preview",
		State: common.StepStateFailed,
		Input: map[string]any{"targetRef": "source.txt"},
		Output: map[string]any{
			"stage": "patch_preview",
			"error": "stale patch",
		},
	}

	items := newRepairSubsystem(nil, workspaceRepo).runtimeRepairPatchContext(ctx, run, failedStep)

	if len(items) != 1 {
		t.Fatalf("patch context = %+v, want one target snapshot", items)
	}
	if items[0]["targetRef"] != "source.txt" || items[0]["exists"] != true || items[0]["contentHash"] == "" {
		t.Fatalf("patch context item = %+v, want current source.txt snapshot", items[0])
	}
	if items[0]["source"] != "current_workspace_file" {
		t.Fatalf("patch context source = %#v, want current_workspace_file", items[0]["source"])
	}
}

func TestRepairSubsystemWorkspaceFileManifestUsesDeliverableDirectory(t *testing.T) {
	ctx := context.Background()
	deliverableDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDir, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatalf("write manifest file: %v", err)
	}
	workspaceRepo := memory.NewWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-repair-manifest")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "repair manifest",
		State:                common.WorkspaceStateActive,
		DeliverableDirectory: deliverableDir,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{ID: "run-repair-manifest", WorkspaceID: workspaceID}

	manifest := newRepairSubsystem(nil, workspaceRepo).workspaceFileManifest(ctx, run)

	if len(manifest) != 1 {
		t.Fatalf("manifest = %+v, want one file", manifest)
	}
	if manifest[0]["path"] != "README.md" || manifest[0]["kind"] != "file" {
		t.Fatalf("manifest item = %+v, want README.md file", manifest[0])
	}
}
