package repository_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestWorkspaceRepositoryContract_Memory(t *testing.T) {
	testWorkspaceRepositoryContract(t, func() repository.WorkspaceRepository {
		return memory.NewWorkspaceRepository()
	})
}

func TestWorkspaceRepositoryContract_File(t *testing.T) {
	testWorkspaceRepositoryContract(t, func() repository.WorkspaceRepository {
		return file.NewWorkspaceRepository(filepath.Join(t.TempDir(), "workspace.json"))
	})
}

func TestRunRepositoryContract_Memory(t *testing.T) {
	testRunRepositoryContract(t, func() repository.RunRepository {
		return memory.NewRunRepository()
	})
}

func TestRunRepositoryContract_File(t *testing.T) {
	testRunRepositoryContract(t, func() repository.RunRepository {
		return file.NewRunRepository(filepath.Join(t.TempDir(), "run.json"))
	})
}

func TestStepRepositoryContract_Memory(t *testing.T) {
	testStepRepositoryContract(t, func() repository.StepRepository {
		return memory.NewStepRepository()
	})
}

func TestStepRepositoryContract_File(t *testing.T) {
	testStepRepositoryContract(t, func() repository.StepRepository {
		return file.NewStepRepository(filepath.Join(t.TempDir(), "step.json"))
	})
}

func TestDecisionTicketRepositoryContract_Memory(t *testing.T) {
	testDecisionTicketRepositoryContract(t, func() repository.DecisionTicketRepository {
		return memory.NewDecisionTicketRepository()
	})
}

func TestDecisionTicketRepositoryContract_File(t *testing.T) {
	testDecisionTicketRepositoryContract(t, func() repository.DecisionTicketRepository {
		return file.NewDecisionTicketRepository(filepath.Join(t.TempDir(), "ticket.json"))
	})
}

func TestDecisionResolutionRepositoryContract_Memory(t *testing.T) {
	testDecisionResolutionRepositoryContract(t, func() repository.DecisionResolutionRepository {
		return memory.NewDecisionResolutionRepository()
	})
}

func TestDecisionResolutionRepositoryContract_File(t *testing.T) {
	testDecisionResolutionRepositoryContract(t, func() repository.DecisionResolutionRepository {
		return file.NewDecisionResolutionRepository(filepath.Join(t.TempDir(), "resolution.json"))
	})
}

func TestApprovalRepositoryContract_Memory(t *testing.T) {
	testApprovalRepositoryContract(t, func() repository.ApprovalRepository {
		return memory.NewApprovalRepository()
	})
}

func TestApprovalRepositoryContract_File(t *testing.T) {
	testApprovalRepositoryContract(t, func() repository.ApprovalRepository {
		return file.NewApprovalRepository(filepath.Join(t.TempDir(), "approval.json"))
	})
}

func testWorkspaceRepositoryContract(t *testing.T, factory func() repository.WorkspaceRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-contract"),
		Title: "workspace-contract",
		State: common.WorkspaceStateActive,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	got.Title = "mutated"

	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get workspace fresh: %v", err)
	}
	if fresh.Title != "workspace-contract" {
		t.Fatalf("clone isolation failed, title = %q", fresh.Title)
	}

	fresh.ActiveRunID = common.RunID("run-1")
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update workspace: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated workspace: %v", err)
	}
	if updated.ActiveRunID != common.RunID("run-1") {
		t.Fatalf("active run id = %q, want run-1", updated.ActiveRunID)
	}

	if _, err := repo.Get(ctx, common.WorkspaceID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &workspacemodel.Workspace{ID: common.WorkspaceID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testRunRepositoryContract(t *testing.T, factory func() repository.RunRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &runmodel.Run{
		ID:          common.RunID("run-contract"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		State:       common.RunStateCreated,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	got.State = common.RunStateCompleted
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update run: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated run: %v", err)
	}
	if updated.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", updated.State)
	}

	if _, err := repo.Get(ctx, common.RunID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &runmodel.Run{ID: common.RunID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testStepRepositoryContract(t *testing.T, factory func() repository.StepRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &runmodel.Step{
		ID:    common.StepID("run-contract/step-1"),
		RunID: common.RunID("run-contract"),
		Kind:  "legacy/minimal",
		State: common.StepStateDispatched,
		Input: map[string]any{"workspaceId": "ws-contract"},
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	got.Input["workspaceId"] = "mutated"
	got.Output = map[string]any{"result": "done"}

	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get fresh step: %v", err)
	}
	if fresh.Input["workspaceId"] != "ws-contract" {
		t.Fatalf("clone isolation failed, workspaceId = %v", fresh.Input["workspaceId"])
	}
	if fresh.Output != nil {
		t.Fatalf("fresh output = %#v, want nil", fresh.Output)
	}

	fresh.State = common.StepStateCompleted
	fresh.Output = map[string]any{"result": "done"}
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update step: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated step: %v", err)
	}
	if updated.State != common.StepStateCompleted {
		t.Fatalf("step state = %q, want completed", updated.State)
	}
	if updated.Output["result"] != "done" {
		t.Fatalf("output result = %v, want done", updated.Output["result"])
	}

	if _, err := repo.Get(ctx, common.StepID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &runmodel.Step{ID: common.StepID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testDecisionTicketRepositoryContract(t *testing.T, factory func() repository.DecisionTicketRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-contract"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract"),
		StepID:      common.StepID("run-contract/manual-gate"),
		Reason:      "manual decision",
		State:       common.DecisionTicketStateOpen,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	got.State = common.DecisionTicketStateResolved
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update ticket: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated ticket: %v", err)
	}
	if updated.State != common.DecisionTicketStateResolved {
		t.Fatalf("ticket state = %q, want resolved", updated.State)
	}

	if _, err := repo.Get(ctx, common.TicketID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &decisionmodel.Ticket{ID: common.TicketID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testDecisionResolutionRepositoryContract(t *testing.T, factory func() repository.DecisionResolutionRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-contract"),
		TicketID: common.TicketID("ticket-contract"),
		Type:     "approve_path",
		Payload:  map[string]any{"operator": "contract-test"},
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create resolution: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.GetByTicketID(ctx, item.TicketID)
	if err != nil {
		t.Fatalf("get resolution by ticket id: %v", err)
	}
	got.Type = "mutated"

	fresh, err := repo.GetByTicketID(ctx, item.TicketID)
	if err != nil {
		t.Fatalf("get resolution by ticket id fresh: %v", err)
	}
	if fresh.Type != "approve_path" {
		t.Fatalf("clone isolation failed, type = %q", fresh.Type)
	}
	if fresh.Payload["operator"] != "contract-test" {
		t.Fatalf("payload operator = %v, want contract-test", fresh.Payload["operator"])
	}

	if _, err := repo.GetByTicketID(ctx, common.TicketID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
}

func testApprovalRepositoryContract(t *testing.T, factory func() repository.ApprovalRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &approvalmodel.Approval{
		ID:          common.ApprovalID("approval-contract"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract"),
		StepID:      common.StepID("run-contract/manual-approval"),
		Reason:      "manual approval",
		State:       common.ApprovalStatePending,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	got.State = common.ApprovalStateApproved
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated approval: %v", err)
	}
	if updated.State != common.ApprovalStateApproved {
		t.Fatalf("approval state = %q, want approved", updated.State)
	}

	if _, err := repo.Get(ctx, common.ApprovalID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &approvalmodel.Approval{ID: common.ApprovalID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}
