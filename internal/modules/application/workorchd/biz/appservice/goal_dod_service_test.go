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

func TestDefineGoalWithBackendMergesWorkspaceGoalAndRemovesOlderDefinitions(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-goal-merge", "goal merge workspace", "", "workspaces/goal-merge")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	first, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "initial goal")
	if err != nil {
		t.Fatalf("first DefineGoalWithBackend: %v", err)
	}
	second, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "added goal")
	if err != nil {
		t.Fatalf("second DefineGoalWithBackend: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("merged goal id = %q, want existing id %q", second.ID, first.ID)
	}
	if second.Text != "initial goal\nadded goal" {
		t.Fatalf("merged goal text = %q, want initial plus added", second.Text)
	}
	list, err := ListGoalsWithBackend(context.Background(), "file", dataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("ListGoalsWithBackend: %v", err)
	}
	if len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("goal list = %+v, want one canonical Goal", list)
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

func TestDefineDoDWithBackendNormalizesMarkdownDoD(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-dod-markdown", "dod markdown workspace", "", "workspaces/dod-markdown")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{`
### 3.1 Functional
- [ ] **Forwarding**: request reaches the configured target
- [ ] **Traceability**:
  - produced evidence is linked to the item
  - final result references the evidence
`})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	want := []string{
		"Functional： Forwarding: request reaches the configured target",
		"Functional / Traceability： produced evidence is linked to the item",
		"Functional / Traceability： final result references the evidence",
	}
	if len(dod.Items) != len(want) {
		t.Fatalf("dod items = %#v, want %#v", dod.Items, want)
	}
	for idx := range want {
		if dod.Items[idx] != want[idx] {
			t.Fatalf("dod items = %#v, want %#v", dod.Items, want)
		}
	}
}

func TestDefineDoDWithBackendMergesWorkspaceDoDAndRemovesOlderDefinitions(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-dod-merge", "dod merge workspace", "", "workspaces/dod-merge")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	first, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"initial dod", "shared dod"})
	if err != nil {
		t.Fatalf("first DefineDoDWithBackend: %v", err)
	}
	second, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"shared dod", "added dod"})
	if err != nil {
		t.Fatalf("second DefineDoDWithBackend: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("merged dod id = %q, want existing id %q", second.ID, first.ID)
	}
	want := []string{"initial dod", "shared dod", "added dod"}
	if len(second.Items) != len(want) {
		t.Fatalf("merged dod items = %#v, want %#v", second.Items, want)
	}
	for idx := range want {
		if second.Items[idx] != want[idx] {
			t.Fatalf("merged dod items = %#v, want %#v", second.Items, want)
		}
	}
	list, err := ListDoDsWithBackend(context.Background(), "file", dataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("ListDoDsWithBackend: %v", err)
	}
	if len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("dod list = %+v, want one canonical DoD", list)
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
