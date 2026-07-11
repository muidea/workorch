package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
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
	if items[0]["targetRef"] != "source.txt" || items[0]["contentHash"] != nil {
		t.Fatalf("patch context item = %+v, want source.txt acquisition placeholder without direct hash", items[0])
	}
	if items[0]["source"] != "runtime_managed_file_read_required" ||
		items[0]["runtimeManaged"] != false ||
		items[0]["runtimeManagedAcquisitionRequired"] != true ||
		items[0]["safeForFileWrite"] != false ||
		items[0]["snapshotValidity"] != "runtime_managed_file_read_required" {
		t.Fatalf("patch context item = %+v, want runtime-managed acquisition required", items[0])
	}
}

func TestRepairSubsystemRuntimeRepairPatchContextPrefersRuntimeManagedReadSnapshot(t *testing.T) {
	ctx := context.Background()
	deliverableDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDir, "source.txt"), []byte("current content\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	workspaceRepo := memory.NewWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	workspaceID := common.WorkspaceID("ws-repair-runtime-managed-context")
	runID := common.RunID("run-repair-runtime-managed-context")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "repair runtime managed context",
		State:                common.WorkspaceStateActive,
		DeliverableDirectory: deliverableDir,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	readStep := &runmodel.Step{
		ID:    "run-repair-runtime-managed-context/read-target-source",
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateSucceeded,
		Input: map[string]any{
			"path":                      "source.txt",
			"targetRef":                 "source.txt",
			"runtimeManagedAcquisition": true,
			"acquisitionKind":           "target_snapshot",
		},
		Output: map[string]any{
			"status":      "completed",
			"stage":       "file_read",
			"path":        "source.txt",
			"exists":      true,
			"content":     "current content\n",
			"contentHash": "hash-source-current",
			"truncated":   false,
			"isBinary":    false,
			"lineCount":   1,
		},
	}
	if err := stepRepo.Create(ctx, readStep); err != nil {
		t.Fatalf("seed read step: %v", err)
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	failedStep := &runmodel.Step{
		ID:    "run-repair-runtime-managed-context/patch-preview",
		RunID: run.ID,
		Kind:  "patch/preview",
		State: common.StepStateFailed,
		Input: map[string]any{"targetRef": "source.txt"},
		Output: map[string]any{
			"stage": "patch_preview",
			"error": "stale patch",
		},
	}

	items := newRepairSubsystem(stepRepo, workspaceRepo).runtimeRepairPatchContext(ctx, run, failedStep)

	if len(items) != 1 {
		t.Fatalf("patch context = %+v, want one target snapshot", items)
	}
	item := items[0]
	if item["source"] != "runtime_managed_file_read" ||
		item["runtimeManaged"] != true ||
		item["sourceStepId"] != string(readStep.ID) ||
		item["contentHash"] != "hash-source-current" ||
		item["safeForFileWrite"] != true ||
		item["safeForExactPatch"] != true {
		t.Fatalf("patch context item = %+v, want runtime-managed safe snapshot", item)
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
