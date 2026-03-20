package memory

import (
	"context"
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceRepository_CRUDAndCloneIsolation(t *testing.T) {
	ctx := context.Background()
	repo := NewWorkspaceRepository()

	original := &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-1"),
		Title: "workspace-1",
		State: common.WorkspaceStateActive,
	}
	if err := repo.Create(ctx, original); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.Create(ctx, original); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("create duplicate workspace err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, original.ID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	got.Title = "mutated"

	gotAgain, err := repo.Get(ctx, original.ID)
	if err != nil {
		t.Fatalf("get workspace again: %v", err)
	}
	if gotAgain.Title != "workspace-1" {
		t.Fatalf("workspace clone isolation failed, title = %q", gotAgain.Title)
	}

	gotAgain.ActiveRunID = common.RunID("run-1")
	if err := repo.Update(ctx, gotAgain); err != nil {
		t.Fatalf("update workspace: %v", err)
	}

	updated, err := repo.Get(ctx, original.ID)
	if err != nil {
		t.Fatalf("get updated workspace: %v", err)
	}
	if updated.ActiveRunID != common.RunID("run-1") {
		t.Fatalf("active run id = %q, want run-1", updated.ActiveRunID)
	}

	if _, err := repo.Get(ctx, common.WorkspaceID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing workspace err = %v, want not found", err)
	}
}

func TestRunRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := NewRunRepository()

	r := &runmodel.Run{
		ID:          common.RunID("run-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		State:       common.RunStateCreated,
	}
	if err := repo.Create(ctx, r); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := repo.Create(ctx, r); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("create duplicate run err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, r.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	got.State = common.RunStateCompleted
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update run: %v", err)
	}

	updated, err := repo.Get(ctx, r.ID)
	if err != nil {
		t.Fatalf("get updated run: %v", err)
	}
	if updated.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", updated.State)
	}

	if _, err := repo.Get(ctx, common.RunID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing run err = %v, want not found", err)
	}
}

func TestStepRepository_CloneIsolation(t *testing.T) {
	ctx := context.Background()
	repo := NewStepRepository()

	step := &runmodel.Step{
		ID:    common.StepID("run-1/step-1"),
		RunID: common.RunID("run-1"),
		Kind:  "legacy/minimal",
		State: common.StepStateDispatched,
		Input: map[string]any{"workspaceId": "ws-1"},
	}
	if err := repo.Create(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}

	got, err := repo.Get(ctx, step.ID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	got.Input["workspaceId"] = "mutated"
	got.Output = map[string]any{"result": "done"}

	fresh, err := repo.Get(ctx, step.ID)
	if err != nil {
		t.Fatalf("get step again: %v", err)
	}
	if fresh.Input["workspaceId"] != "ws-1" {
		t.Fatalf("step input clone isolation failed, workspaceId = %v", fresh.Input["workspaceId"])
	}
	if fresh.Output != nil {
		t.Fatalf("step output should still be nil, got %#v", fresh.Output)
	}

	fresh.State = common.StepStateCompleted
	fresh.Output = map[string]any{"result": "done"}
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update step: %v", err)
	}

	updated, err := repo.Get(ctx, step.ID)
	if err != nil {
		t.Fatalf("get updated step: %v", err)
	}
	if updated.State != common.StepStateCompleted {
		t.Fatalf("step state = %q, want completed", updated.State)
	}
	if updated.Output["result"] != "done" {
		t.Fatalf("step output result = %v, want done", updated.Output["result"])
	}
}
