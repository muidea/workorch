package activationports

import (
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceExecutionActivationSnapshotProjectsActivationResult(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{
		ID:          "ws-activation-snapshot",
		Title:       "activation snapshot",
		State:       common.WorkspaceStateActive,
		ActiveRunID: "run-activation-snapshot",
	}
	run := &runmodel.Run{
		ID:          "run-activation-snapshot",
		WorkspaceID: workspace.ID,
		GoalID:      "goal-activation-snapshot",
		DoDID:       "dod-activation-snapshot",
		State:       common.RunStateActive,
	}
	goal := &workspacemodel.Goal{ID: run.GoalID, WorkspaceID: workspace.ID, Text: "execute activation snapshot"}
	dod := &workspacemodel.DoD{ID: run.DoDID, WorkspaceID: workspace.ID, Items: []string{"activation snapshot is projected"}}

	got := WorkspaceExecutionActivationSnapshot(workspace, run, goal, dod)
	if got.Workspace.ID != string(workspace.ID) {
		t.Fatalf("workspace ID = %q, want %q", got.Workspace.ID, workspace.ID)
	}
	if got.Run.ID != string(run.ID) || got.Run.WorkspaceID != string(workspace.ID) {
		t.Fatalf("run = %+v, want run/workspace IDs", got.Run)
	}
	if got.Snapshot.Workspace.ID != got.Workspace.ID {
		t.Fatalf("snapshot workspace ID = %q, want activation workspace ID %q", got.Snapshot.Workspace.ID, got.Workspace.ID)
	}
	if got.Snapshot.Run == nil || got.Snapshot.Run.ID != got.Run.ID {
		t.Fatalf("snapshot run = %+v, want activation run %q", got.Snapshot.Run, got.Run.ID)
	}
	if len(got.Snapshot.Goals) != 1 || got.Snapshot.Goals[0].ID != string(goal.ID) {
		t.Fatalf("snapshot goals = %+v, want seeded goal", got.Snapshot.Goals)
	}
	if len(got.Snapshot.DoDs) != 1 || got.Snapshot.DoDs[0].ID != string(dod.ID) {
		t.Fatalf("snapshot dods = %+v, want seeded DoD", got.Snapshot.DoDs)
	}
}
