package orchestrator

import (
	"context"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRunStateSubsystemReconcileWorkspaceActiveRunClearsMissingActiveRun(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	subsystem := newRunStateSubsystem(runRepo, workspaceRepo)
	workspaceID := common.WorkspaceID("ws-missing-active-run")

	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: "run-missing-active",
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	ws, activeRunID, changed, err := subsystem.reconcileWorkspaceActiveRun(ctx, workspaceID)
	if err != nil {
		t.Fatalf("reconcile workspace active run: %v", err)
	}
	if !changed || activeRunID != "" {
		t.Fatalf("changed=%v activeRunID=%q, want changed with empty event run id", changed, activeRunID)
	}
	if ws.ActiveRunID != "" || ws.State != common.WorkspaceStateActive {
		t.Fatalf("workspace = %+v, want active with no active run", ws)
	}
	persisted, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if persisted.ActiveRunID != "" || persisted.State != common.WorkspaceStateActive {
		t.Fatalf("persisted workspace = %+v, want active with no active run", persisted)
	}
}
