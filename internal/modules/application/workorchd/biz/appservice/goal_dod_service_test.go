package appservice

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestUpdateGoalWithBackendUpdatesPersistedGoal(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-goal-update", "goal update workspace", "", "workspaces/goal-update")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "initial goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}

	updated, err := UpdateGoalWithBackend(context.Background(), "file", dataRoot, goal.ID, "updated goal")
	if err != nil {
		t.Fatalf("UpdateGoalWithBackend: %v", err)
	}
	if updated.ID != goal.ID || updated.WorkspaceID != workspace.ID || updated.Text != "updated goal" {
		t.Fatalf("updated goal = %+v, want updated goal in same workspace", updated)
	}
	read, err := ReadGoalWithBackend(context.Background(), "file", dataRoot, goal.ID)
	if err != nil {
		t.Fatalf("ReadGoalWithBackend: %v", err)
	}
	if read.Text != "updated goal" {
		t.Fatalf("read goal text = %q, want updated goal", read.Text)
	}
}

func TestUpdateGoalWithBackendValidatesInput(t *testing.T) {
	if _, err := UpdateGoalWithBackend(context.Background(), "file", t.TempDir(), "", "updated"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty goal id err = %v, want invalid argument", err)
	}
	if _, err := UpdateGoalWithBackend(context.Background(), "file", t.TempDir(), "goal-missing", ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty goal text err = %v, want invalid argument", err)
	}
	if _, err := UpdateGoalWithBackend(context.Background(), "file", t.TempDir(), "goal-missing", "updated"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing goal err = %v, want not found", err)
	}
}

func TestUpdateDoDWithBackendUpdatesPersistedDoD(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-dod-update", "dod update workspace", "", "workspaces/dod-update")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"initial dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	updated, err := UpdateDoDWithBackend(context.Background(), "file", dataRoot, dod.ID, []string{"updated dod", "second dod"})
	if err != nil {
		t.Fatalf("UpdateDoDWithBackend: %v", err)
	}
	if updated.ID != dod.ID || updated.WorkspaceID != workspace.ID || updated.State != string(common.DoDStateReady) {
		t.Fatalf("updated dod = %+v, want same DoD reset to ready", updated)
	}
	if len(updated.Items) != 2 || updated.Items[0] != "updated dod" || updated.Items[1] != "second dod" {
		t.Fatalf("updated dod items = %+v, want updated items", updated.Items)
	}
}

func TestUpdateDoDWithBackendValidatesInput(t *testing.T) {
	if _, err := UpdateDoDWithBackend(context.Background(), "file", t.TempDir(), "", []string{"updated"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty dod id err = %v, want invalid argument", err)
	}
	if _, err := UpdateDoDWithBackend(context.Background(), "file", t.TempDir(), "dod-missing", nil); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty dod items err = %v, want invalid argument", err)
	}
	if _, err := UpdateDoDWithBackend(context.Background(), "file", t.TempDir(), "dod-missing", []string{"updated"}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing dod err = %v, want not found", err)
	}
}
