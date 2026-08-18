package appservice

import (
	"context"
	"errors"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
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

	updated, err := UpdateGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, goal.ID, "updated goal")
	if err != nil {
		t.Fatalf("UpdateGoalWithBackend: %v", err)
	}
	if updated.ID != goal.ID || updated.WorkspaceID != workspace.ID || updated.Text != "- updated goal" {
		t.Fatalf("updated goal = %+v, want updated goal in same workspace", updated)
	}
	read, err := ReadGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, goal.ID)
	if err != nil {
		t.Fatalf("ReadGoalWithBackend: %v", err)
	}
	if read.Text != "- updated goal" {
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
	if second.Text != "- initial goal\n- added goal" {
		t.Fatalf("merged goal text = %q, want initial plus structured added goal", second.Text)
	}
	list, err := ListGoalsWithBackend(context.Background(), "file", dataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("ListGoalsWithBackend: %v", err)
	}
	if len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("goal list = %+v, want one canonical Goal", list)
	}
}

func TestDefineGoalWithBackendFormatsAddedGoalAsIndependentItem(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-goal-format", "goal format workspace", "", "workspaces/goal-format")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	first, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "### Existing\n- keep current behavior")
	if err != nil {
		t.Fatalf("first DefineGoalWithBackend: %v", err)
	}
	second, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "preserve each new requirement as an independent coverage target")
	if err != nil {
		t.Fatalf("second DefineGoalWithBackend: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("merged goal id = %q, want existing id %q", second.ID, first.ID)
	}
	want := "### Existing\n- keep current behavior\n- preserve each new requirement as an independent coverage target"
	if second.Text != want {
		t.Fatalf("merged goal text = %q, want %q", second.Text, want)
	}
}

func TestDefineGoalWithBackendFormatsPlainGoalText(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-goal-plain-format", "goal plain format workspace", "", "workspaces/goal-plain-format")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "first target；second target\nthird target")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	want := "- first target\n- second target\n- third target"
	if goal.Text != want {
		t.Fatalf("goal text = %q, want %q", goal.Text, want)
	}
}

func TestDefineGoalWithBackendPreservesStructuredMarkdownGoalText(t *testing.T) {
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-goal-md-format", "goal markdown format workspace", "", "workspaces/goal-md-format")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "### Goals  \n\n- first target  \n\n\n- second target")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	want := "### Goals\n\n- first target\n\n- second target"
	if goal.Text != want {
		t.Fatalf("goal text = %q, want %q", goal.Text, want)
	}
}

func TestDefineGoalWithBackendReopensCompletedWorkspaceAndInvalidatesOldOutcome(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-goal-reopen", "goal reopen workspace", "", "workspaces/goal-reopen")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "initial goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend initial: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"initial dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend initial: %v", err)
	}
	runID := common.RunID("run-old-accepted")
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: common.WorkspaceID(workspace.ID),
		GoalID:      common.GoalID(goal.ID),
		DoDID:       common.DoDID(dod.ID),
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID(string(runID) + "/result/dod-assessment"),
		WorkspaceID: common.WorkspaceID(workspace.ID),
		RunID:       runID,
		GoalID:      common.GoalID(goal.ID),
		DoDID:       common.DoDID(dod.ID),
		State:       common.ResultStateAccepted,
		Summary:     "old accepted result",
		CreatedAt:   "2026-05-18T10:00:00Z",
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	ws, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	ws.State = common.WorkspaceStateActive
	if err := exports.Repositories.Workspaces.Update(ctx, ws); err != nil {
		t.Fatalf("mark workspace completed: %v", err)
	}

	if _, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "new goal requirement"); err != nil {
		t.Fatalf("DefineGoalWithBackend new: %v", err)
	}
	snapshot, err := ReadWorkspaceSnapshotWithBackend(ctx, "file", dataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateActive) || snapshot.Workspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want active without active run after new goal", snapshot.Workspace)
	}
	if snapshot.Outcome.Accepted || snapshot.Outcome.Terminal || snapshot.Outcome.ResultID != "" {
		t.Fatalf("outcome = %+v, want old accepted result invalidated by definition change", snapshot.Outcome)
	}
}

func TestUpdateGoalWithBackendValidatesInput(t *testing.T) {
	if _, err := UpdateGoalWithBackend(context.Background(), "file", t.TempDir(), "ws-update-validation", "", "updated"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty goal id err = %v, want invalid argument", err)
	}
	if _, err := UpdateGoalWithBackend(context.Background(), "file", t.TempDir(), "ws-update-validation", "goal-missing", ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty goal text err = %v, want invalid argument", err)
	}
	if _, err := UpdateGoalWithBackend(context.Background(), "file", t.TempDir(), "ws-update-validation", "goal-missing", "updated"); !errors.Is(err, common.ErrNotFound) {
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

	updated, err := UpdateDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, dod.ID, []string{"updated dod", "second dod"})
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
	if _, err := UpdateDoDWithBackend(context.Background(), "file", t.TempDir(), "ws-update-validation", "", []string{"updated"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty dod id err = %v, want invalid argument", err)
	}
	if _, err := UpdateDoDWithBackend(context.Background(), "file", t.TempDir(), "ws-update-validation", "dod-missing", nil); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty dod items err = %v, want invalid argument", err)
	}
	if _, err := UpdateDoDWithBackend(context.Background(), "file", t.TempDir(), "ws-update-validation", "dod-missing", []string{"updated"}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing dod err = %v, want not found", err)
	}
}
