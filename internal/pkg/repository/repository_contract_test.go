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

func TestRepositoryBundleContract_Memory(t *testing.T) {
	bundle := repositoryBundle{
		workspaces:  memory.NewWorkspaceRepository(),
		runs:        memory.NewRunRepository(),
		steps:       memory.NewStepRepository(),
		tickets:     memory.NewDecisionTicketRepository(),
		resolutions: memory.NewDecisionResolutionRepository(),
		approvals:   memory.NewApprovalRepository(),
	}
	testRepositoryBundleContract(t, bundle, func() repositoryBundle { return bundle })
}

func TestRepositoryBundleContract_File(t *testing.T) {
	dir := t.TempDir()
	build := func() repositoryBundle {
		return repositoryBundle{
			workspaces:  file.NewWorkspaceRepository(filepath.Join(dir, "workspace.json")),
			runs:        file.NewRunRepository(filepath.Join(dir, "run.json")),
			steps:       file.NewStepRepository(filepath.Join(dir, "step.json")),
			tickets:     file.NewDecisionTicketRepository(filepath.Join(dir, "ticket.json")),
			resolutions: file.NewDecisionResolutionRepository(filepath.Join(dir, "resolution.json")),
			approvals:   file.NewApprovalRepository(filepath.Join(dir, "approval.json")),
		}
	}
	testRepositoryBundleContract(t, build(), build)
}

type repositoryBundle struct {
	workspaces  repository.WorkspaceRepository
	runs        repository.RunRepository
	steps       repository.StepRepository
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	approvals   repository.ApprovalRepository
}

func testWorkspaceRepositoryContract(t *testing.T, factory func() repository.WorkspaceRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &workspacemodel.Workspace{
		ID:               common.WorkspaceID("ws-contract"),
		Title:            "workspace-contract",
		Description:      "workspace contract description",
		WorkingDirectory: "/tmp/workorch/ws-contract",
		State:            common.WorkspaceStateActive,
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
	if fresh.Description != "workspace contract description" {
		t.Fatalf("description = %q, want workspace contract description", fresh.Description)
	}
	if fresh.WorkingDirectory != "/tmp/workorch/ws-contract" {
		t.Fatalf("workingDirectory = %q, want /tmp/workorch/ws-contract", fresh.WorkingDirectory)
	}

	fresh.ActiveRunID = common.RunID("run-1")
	fresh.Description = "workspace contract updated description"
	fresh.WorkingDirectory = "/tmp/workorch/ws-contract-updated"
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
	if updated.Description != "workspace contract updated description" {
		t.Fatalf("updated description = %q, want workspace contract updated description", updated.Description)
	}
	if updated.WorkingDirectory != "/tmp/workorch/ws-contract-updated" {
		t.Fatalf("updated workingDirectory = %q, want /tmp/workorch/ws-contract-updated", updated.WorkingDirectory)
	}

	second := &workspacemodel.Workspace{
		ID:               common.WorkspaceID("ws-contract-2"),
		Title:            "workspace-contract-2",
		WorkingDirectory: "/tmp/workorch/ws-contract-2",
		State:            common.WorkspaceStateActive,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second workspace: %v", err)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, want 2", len(listed))
	}
	if listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed ids = %q/%q, want %q/%q", listed[0].ID, listed[1].ID, item.ID, second.ID)
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
	updated.FailureReason = "manual fail"
	if err := repo.Update(ctx, updated); err != nil {
		t.Fatalf("update run failure reason: %v", err)
	}

	refreshed, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get refreshed run: %v", err)
	}
	if refreshed.FailureReason != "manual fail" {
		t.Fatalf("failure reason = %q, want manual fail", refreshed.FailureReason)
	}

	second := &runmodel.Run{
		ID:          common.RunID("run-contract-2"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		State:       common.RunStateCreated,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second run: %v", err)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, want 2", len(listed))
	}
	if listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed ids = %q/%q, want %q/%q", listed[0].ID, listed[1].ID, item.ID, second.ID)
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

	if err := repo.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-contract/step-2"),
		RunID: common.RunID("run-contract"),
		Kind:  "legacy/minimal",
		State: common.StepStateCompleted,
		Input: map[string]any{"workspaceId": "ws-contract", "previousStepId": "run-contract/step-1"},
	}); err != nil {
		t.Fatalf("create second step: %v", err)
	}
	if err := repo.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-other/step-1"),
		RunID: common.RunID("run-other"),
		Kind:  "legacy/minimal",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("create other run step: %v", err)
	}

	steps, err := repo.ListByRun(ctx, common.RunID("run-contract"))
	if err != nil {
		t.Fatalf("list steps by run: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("list steps len = %d, want 2", len(steps))
	}
	if steps[0].ID != common.StepID("run-contract/step-1") || steps[1].ID != common.StepID("run-contract/step-2") {
		t.Fatalf("listed step ids = [%s %s], want [run-contract/step-1 run-contract/step-2]", steps[0].ID, steps[1].ID)
	}
	steps[0].Output["result"] = "mutated"
	refreshed, err := repo.Get(ctx, common.StepID("run-contract/step-1"))
	if err != nil {
		t.Fatalf("get refreshed step: %v", err)
	}
	if refreshed.Output["result"] != "done" {
		t.Fatalf("refreshed output result = %v, want done", refreshed.Output["result"])
	}

	steps, err = repo.ListByRun(ctx, common.RunID("run-missing"))
	if err != nil {
		t.Fatalf("list missing run steps: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("missing run steps len = %d, want 0", len(steps))
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

	second := &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-contract-2"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract-2"),
		StepID:      common.StepID("run-contract-2/manual-gate"),
		Reason:      "manual decision 2",
		State:       common.DecisionTicketStateOpen,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second ticket: %v", err)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, want 2", len(listed))
	}
	if listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed ids = %q/%q, want %q/%q", listed[0].ID, listed[1].ID, item.ID, second.ID)
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

	second := &approvalmodel.Approval{
		ID:          common.ApprovalID("approval-contract-2"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract-2"),
		StepID:      common.StepID("run-contract-2/manual-approval"),
		Reason:      "manual approval 2",
		State:       common.ApprovalStatePending,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second approval: %v", err)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, want 2", len(listed))
	}
	if listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed ids = %q/%q, want %q/%q", listed[0].ID, listed[1].ID, item.ID, second.ID)
	}

	if _, err := repo.Get(ctx, common.ApprovalID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &approvalmodel.Approval{ID: common.ApprovalID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testRepositoryBundleContract(t *testing.T, bundle repositoryBundle, reopen func() repositoryBundle) {
	t.Helper()

	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-bundle-contract")
	runID := common.RunID("run-bundle-contract")
	stepID := common.StepID("run-bundle-contract/step-1")
	ticketID := common.TicketID("ticket-bundle-contract")
	resolutionID := common.ResolutionID("resolution-bundle-contract")
	approvalID := common.ApprovalID("approval-bundle-contract")

	workspace := &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "bundle contract workspace",
		State: common.WorkspaceStateActive,
	}
	if err := bundle.workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := bundle.workspaces.Create(ctx, workspace); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate workspace create err = %v, want conflict", err)
	}

	run := &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		State:              common.RunStateBlockedOnDecision,
		BlockingTicketID:   ticketID,
		BlockingApprovalID: approvalID,
	}
	if err := bundle.runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := bundle.runs.Create(ctx, run); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate run create err = %v, want conflict", err)
	}

	step := &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "legacy/manual",
		State: common.StepStateDispatched,
		Input: map[string]any{"workspaceId": string(workspaceID), "runId": string(runID)},
	}
	if err := bundle.steps.Create(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := bundle.steps.Create(ctx, step); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate step create err = %v, want conflict", err)
	}
	step.State = common.StepStateCompleted
	step.Output = map[string]any{"status": "completed", "ticketId": string(ticketID)}
	if err := bundle.steps.Update(ctx, step); err != nil {
		t.Fatalf("update step: %v", err)
	}

	ticket := &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      "bundle manual decision",
		State:       common.DecisionTicketStateOpen,
	}
	if err := bundle.tickets.Create(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := bundle.tickets.Create(ctx, ticket); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate ticket create err = %v, want conflict", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := bundle.tickets.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}

	resolution := &decisionmodel.Resolution{
		ID:       resolutionID,
		TicketID: ticketID,
		Type:     "approve_path",
		Payload:  map[string]any{"operator": "bundle-test"},
	}
	if err := bundle.resolutions.Create(ctx, resolution); err != nil {
		t.Fatalf("create resolution: %v", err)
	}
	if err := bundle.resolutions.Create(ctx, resolution); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate resolution create err = %v, want conflict", err)
	}

	approval := &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      "bundle manual approval",
		State:       common.ApprovalStatePending,
	}
	if err := bundle.approvals.Create(ctx, approval); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if err := bundle.approvals.Create(ctx, approval); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate approval create err = %v, want conflict", err)
	}
	approval.State = common.ApprovalStateRejected
	if err := bundle.approvals.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	workspace.ActiveRunID = runID
	if err := bundle.workspaces.Update(ctx, workspace); err != nil {
		t.Fatalf("update workspace: %v", err)
	}

	run.State = common.RunStateFailed
	run.FailureReason = "approval rejected"
	if err := bundle.runs.Update(ctx, run); err != nil {
		t.Fatalf("update run: %v", err)
	}

	reloaded := reopen()

	reloadedWorkspace, err := reloaded.workspaces.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if reloadedWorkspace.ActiveRunID != runID {
		t.Fatalf("reloaded workspace activeRunID = %q, want %q", reloadedWorkspace.ActiveRunID, runID)
	}

	reloadedRun, err := reloaded.runs.Get(ctx, runID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if reloadedRun.State != common.RunStateFailed || reloadedRun.FailureReason != "approval rejected" {
		t.Fatalf("reloaded run = %+v, want failed with reason", reloadedRun)
	}
	if reloadedRun.BlockingTicketID != ticketID || reloadedRun.BlockingApprovalID != approvalID {
		t.Fatalf("reloaded run blocking refs = %+v, want %q/%q", reloadedRun, ticketID, approvalID)
	}

	reloadedSteps, err := reloaded.steps.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("reload steps by run: %v", err)
	}
	if len(reloadedSteps) != 1 || reloadedSteps[0].Output["status"] != "completed" {
		t.Fatalf("reloaded steps = %+v, want completed output", reloadedSteps)
	}
	reloadedSteps[0].Output["status"] = "mutated"
	stepFresh, err := reloaded.steps.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("reload step fresh: %v", err)
	}
	if stepFresh.Output["status"] != "completed" {
		t.Fatalf("step output status = %v, want completed", stepFresh.Output["status"])
	}

	reloadedTickets, err := reloaded.tickets.List(ctx)
	if err != nil {
		t.Fatalf("reload tickets: %v", err)
	}
	if len(reloadedTickets) != 1 || reloadedTickets[0].State != common.DecisionTicketStateResolved {
		t.Fatalf("reloaded tickets = %+v, want one resolved ticket", reloadedTickets)
	}

	reloadedResolution, err := reloaded.resolutions.GetByTicketID(ctx, ticketID)
	if err != nil {
		t.Fatalf("reload resolution: %v", err)
	}
	if reloadedResolution.ID != resolutionID || reloadedResolution.Payload["operator"] != "bundle-test" {
		t.Fatalf("reloaded resolution = %+v, want %q with payload", reloadedResolution, resolutionID)
	}

	reloadedApprovals, err := reloaded.approvals.List(ctx)
	if err != nil {
		t.Fatalf("reload approvals: %v", err)
	}
	if len(reloadedApprovals) != 1 || reloadedApprovals[0].State != common.ApprovalStateRejected {
		t.Fatalf("reloaded approvals = %+v, want one rejected approval", reloadedApprovals)
	}

	if _, err := reloaded.workspaces.Get(ctx, common.WorkspaceID("workspace-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing workspace get err = %v, want not found", err)
	}
	if _, err := reloaded.runs.Get(ctx, common.RunID("run-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing run get err = %v, want not found", err)
	}
	if _, err := reloaded.steps.Get(ctx, common.StepID("step-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing step get err = %v, want not found", err)
	}
	if _, err := reloaded.tickets.Get(ctx, common.TicketID("ticket-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing ticket get err = %v, want not found", err)
	}
	if _, err := reloaded.resolutions.GetByTicketID(ctx, common.TicketID("ticket-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing resolution get err = %v, want not found", err)
	}
	if _, err := reloaded.approvals.Get(ctx, common.ApprovalID("approval-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing approval get err = %v, want not found", err)
	}
}
