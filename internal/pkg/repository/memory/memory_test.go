package memory

import (
	"context"
	"errors"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustMemoryStepPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

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

func TestWorkspaceRepositoryClearsTransientAuthoringSummary(t *testing.T) {
	ctx := context.Background()
	repo := NewWorkspaceRepository()
	item := &workspacemodel.Workspace{ID: "ws-transient-summary", AuthoringSummary: &workspacemodel.AuthoringSummary{State: "failed", FailureCode: "stale"}}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	if item.AuthoringSummary == nil {
		t.Fatal("repository unexpectedly mutated the caller's transient summary")
	}
	stored, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AuthoringSummary != nil {
		t.Fatalf("memory repository restored transient summary: %+v", stored.AuthoringSummary)
	}
}
func TestRunRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := NewRunRepository()
	r := &runmodel.Run{
		ID:          common.RunID("run-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		State:       common.RunStatePending,
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

func TestRunRepositoryRejectsAmbiguousBootstrapAuthorityOnWriteAndRead(t *testing.T) {
	ctx := context.Background()
	repo := NewRunRepository()
	policy := runtimecontract.RuntimeEvidencePolicy{ID: "policy-1", AppliesTo: runtimecontract.StepKindFileWrite}.Normalized()
	invalidRun := func(id common.RunID) *runmodel.Run {
		bootstrap := runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{policy, policy},
		}}
		return &runmodel.Run{ID: id, WorkspaceID: "ws-raw-bootstrap", State: common.RunStatePending, RuntimeBootstrap: &bootstrap}
	}

	if err := repo.Create(ctx, invalidRun("run-write")); err == nil {
		t.Fatal("memory Run repository accepted duplicate bootstrap authority on write")
	}

	stored := invalidRun("run-read")
	stored.Revision = 1
	stored.WorkspaceRunGeneration = 1
	repo.mu.Lock()
	repo.items[workspaceScopedKey(stored.WorkspaceID, string(stored.ID))] = stored
	repo.mu.Unlock()
	if _, err := repo.Get(ctx, stored.ID); err == nil {
		t.Fatal("memory Run repository accepted duplicate bootstrap authority on read")
	}
}

func TestStepRepository_CloneIsolation(t *testing.T) {
	ctx := context.Background()
	repo := NewStepRepository()

	step := &runmodel.Step{
		ID:    common.StepID("run-1/step-1"),
		RunID: common.RunID("run-1"),
		Kind:  "llm/workspace_execution",
		State: common.StepStateExecuting,
		Input: mustMemoryStepPayload(t, map[string]any{"workspaceId": "ws-1"}),
	}
	if err := repo.Create(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}

	got, err := repo.Get(ctx, step.ID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	got.Input.Fields[0].Value.String = "mutated"
	got.Output = mustMemoryStepPayload(t, map[string]any{"result": "done"})

	fresh, err := repo.Get(ctx, step.ID)
	if err != nil {
		t.Fatalf("get step again: %v", err)
	}
	workspaceID, _ := fresh.Input.String("workspaceId")
	if workspaceID != "ws-1" {
		t.Fatalf("step input clone isolation failed, workspaceId = %v", workspaceID)
	}
	if len(fresh.Output.Fields) != 0 {
		t.Fatalf("step output should still be nil, got %#v", fresh.Output)
	}

	fresh.State = common.StepStateSucceeded
	fresh.Output = mustMemoryStepPayload(t, map[string]any{"result": "done"})
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update step: %v", err)
	}

	updated, err := repo.Get(ctx, step.ID)
	if err != nil {
		t.Fatalf("get updated step: %v", err)
	}
	if updated.State != common.StepStateSucceeded {
		t.Fatalf("step state = %q, want completed", updated.State)
	}
	result, _ := updated.Output.String("result")
	if result != "done" {
		t.Fatalf("step output result = %v, want done", result)
	}
}

func TestMemoryRunAndStepRepositoriesRequireScopedLookupForDuplicateWorkspaceLocalIDs(t *testing.T) {
	ctx := context.Background()
	runs := NewRunRepository()
	steps := NewStepRepository()
	for _, workspaceID := range []common.WorkspaceID{"ws-memory-a", "ws-memory-b"} {
		scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := runs.Create(scoped, &runmodel.Run{
			ID:          "run-shared",
			WorkspaceID: workspaceID,
			State:       common.RunStateActive,
		}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
		if err := steps.Create(scoped, &runmodel.Step{
			ID:    "run-shared/step-001",
			RunID: "run-shared",
			Kind:  "llm/workspace_execution",
			State: common.StepStateSucceeded,
		}); err != nil {
			t.Fatalf("create step %s: %v", workspaceID, err)
		}
	}

	if _, err := runs.Get(ctx, "run-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate run get err = %v, want conflict", err)
	}
	if _, err := steps.Get(ctx, "run-shared/step-001"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate step get err = %v, want conflict", err)
	}
	if _, err := steps.ListByRun(ctx, "run-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate step list err = %v, want conflict", err)
	}

	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-memory-b")
	runItem, err := runs.Get(scoped, "run-shared")
	if err != nil {
		t.Fatalf("scoped run get: %v", err)
	}
	if runItem.WorkspaceID != "ws-memory-b" {
		t.Fatalf("scoped run workspace = %q, want ws-memory-b", runItem.WorkspaceID)
	}
	stepItems, err := steps.ListByRun(scoped, "run-shared")
	if err != nil {
		t.Fatalf("scoped step list: %v", err)
	}
	if len(stepItems) != 1 || stepItems[0].ID != "run-shared/step-001" {
		t.Fatalf("scoped steps = %#v, want one scoped step", stepItems)
	}
}

func TestMemoryGovernanceRepositoriesRequireScopedLookupForDuplicateWorkspaceLocalIDs(t *testing.T) {
	ctx := context.Background()
	tickets := NewDecisionTicketRepository()
	resolutions := NewDecisionResolutionRepository()
	approvals := NewApprovalRepository()
	for _, workspaceID := range []common.WorkspaceID{"ws-gov-a", "ws-gov-b"} {
		scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := tickets.Create(scoped, &decisionmodel.Ticket{
			ID:          "ticket-shared",
			WorkspaceID: workspaceID,
			RunID:       "run-001",
			State:       common.DecisionTicketStateOpen,
			Reason:      "scope regression",
		}); err != nil {
			t.Fatalf("create ticket %s: %v", workspaceID, err)
		}
		if err := resolutions.Create(scoped, &decisionmodel.Resolution{
			ID:       common.ResolutionID("resolution-" + workspaceID),
			TicketID: "ticket-shared",
			Type:     "accepted",
		}); err != nil {
			t.Fatalf("create resolution %s: %v", workspaceID, err)
		}
		if err := approvals.Create(scoped, &approvalmodel.Approval{
			ID:          "approval-shared",
			WorkspaceID: workspaceID,
			RunID:       "run-001",
			State:       common.ApprovalStatePending,
			Reason:      "scope regression",
			Action:      "approve",
		}); err != nil {
			t.Fatalf("create approval %s: %v", workspaceID, err)
		}
	}

	if _, err := tickets.Get(ctx, "ticket-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate ticket get err = %v, want conflict", err)
	}
	if _, err := resolutions.GetByTicketID(ctx, "ticket-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate resolution get err = %v, want conflict", err)
	}
	if _, err := approvals.Get(ctx, "approval-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate approval get err = %v, want conflict", err)
	}

	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-gov-a")
	ticket, err := tickets.Get(scoped, "ticket-shared")
	if err != nil {
		t.Fatalf("scoped ticket get: %v", err)
	}
	if ticket.WorkspaceID != "ws-gov-a" {
		t.Fatalf("scoped ticket workspace = %q, want ws-gov-a", ticket.WorkspaceID)
	}
	resolution, err := resolutions.GetByTicketID(scoped, "ticket-shared")
	if err != nil {
		t.Fatalf("scoped resolution get: %v", err)
	}
	if resolution.ID != "resolution-ws-gov-a" {
		t.Fatalf("scoped resolution id = %q, want resolution-ws-gov-a", resolution.ID)
	}
	approval, err := approvals.Get(scoped, "approval-shared")
	if err != nil {
		t.Fatalf("scoped approval get: %v", err)
	}
	if approval.WorkspaceID != "ws-gov-a" {
		t.Fatalf("scoped approval workspace = %q, want ws-gov-a", approval.WorkspaceID)
	}
}

func TestMemoryApprovalRepositoryScopesDuplicateIDsByWorkspaceAndRun(t *testing.T) {
	ctx := context.Background()
	repo := NewApprovalRepository()
	const (
		workspaceID = common.WorkspaceID("ws-approval-run-scope")
		approvalID  = common.ApprovalID("approval-shared-across-runs")
	)
	for _, runID := range []common.RunID{"run-approval-a", "run-approval-b"} {
		if err := repo.Create(ctx, &approvalmodel.Approval{
			ID: approvalID, WorkspaceID: workspaceID, RunID: runID,
			State: common.ApprovalStatePending, Reason: "run-scoped approval",
		}); err != nil {
			t.Fatalf("create approval for %s: %v", runID, err)
		}
	}
	workspaceCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	wrongWorkspaceCtx := common.ContextWithRuntimeWorkspaceID(ctx, "ws-approval-wrong-scope")
	if _, err := repo.Get(workspaceCtx, approvalID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("workspace-only duplicate get err = %v, want conflict", err)
	}
	first, err := repo.GetScoped(workspaceCtx, workspaceID, "run-approval-a", approvalID)
	if err != nil {
		t.Fatal(err)
	}
	first.State = common.ApprovalStateApproved
	if err := repo.Update(wrongWorkspaceCtx, first); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong workspace context update err = %v, want conflict", err)
	}
	if err := repo.Update(workspaceCtx, first); err != nil {
		t.Fatal(err)
	}
	second, err := repo.GetScoped(workspaceCtx, workspaceID, "run-approval-b", approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if second.State != common.ApprovalStatePending {
		t.Fatalf("second run approval state = %s, want pending", second.State)
	}
	if err := repo.DeleteScoped(workspaceCtx, workspaceID, "run-approval-a", approvalID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetScoped(workspaceCtx, workspaceID, "run-approval-b", approvalID); err != nil {
		t.Fatalf("delete in first run removed second run approval: %v", err)
	}
}

func TestMemoryWorkspaceLocalRepositoriesRequireScopedLookup(t *testing.T) {
	ctx := context.Background()
	goals := NewGoalRepository()
	dods := NewDoDRepository()
	memories := NewMemoryItemRepository()
	results := NewResultRepository()
	artifacts := NewArtifactRepository()
	events := NewEventRecordRepository()
	for _, workspaceID := range []common.WorkspaceID{"ws-local-a", "ws-local-b"} {
		scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := goals.Create(scoped, &workspacemodel.Goal{ID: "goal-shared", WorkspaceID: workspaceID, Text: "goal"}); err != nil {
			t.Fatalf("create goal %s: %v", workspaceID, err)
		}
		if err := dods.Create(scoped, &workspacemodel.DoD{ID: "dod-shared", WorkspaceID: workspaceID, Items: []string{"dod"}, State: "ready"}); err != nil {
			t.Fatalf("create dod %s: %v", workspaceID, err)
		}
		if err := memories.Create(scoped, &memorymodel.MemoryItem{ID: "memory-shared", WorkspaceID: workspaceID, RunID: "run-shared", Content: "memory"}); err != nil {
			t.Fatalf("create memory %s: %v", workspaceID, err)
		}
		if err := results.Create(scoped, &resultmodel.Result{ID: "result-shared", WorkspaceID: workspaceID, RunID: "run-shared", State: common.ResultStateAccepted}); err != nil {
			t.Fatalf("create result %s: %v", workspaceID, err)
		}
		if err := artifacts.Create(scoped, &resultmodel.Artifact{ID: "artifact-shared", WorkspaceID: workspaceID, RunID: "run-shared", ResultID: "result-shared", State: common.ArtifactStateAvailable}); err != nil {
			t.Fatalf("create artifact %s: %v", workspaceID, err)
		}
		if err := events.Append(scoped, &eventstreammodel.EventRecord{
			ID: "event-shared", Version: "workorch.event", Topic: "/test", OccurredAt: "2026-08-30T00:00:00Z",
			SourceModule: "blocks/test", SubjectType: "test", SubjectID: "event-shared", WorkspaceID: workspaceID,
			RunID: "run-shared", Revision: 1, StreamPosition: 1,
		}); err != nil {
			t.Fatalf("append event %s: %v", workspaceID, err)
		}
	}

	if _, err := goals.Get(ctx, "goal-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate goal get err = %v, want conflict", err)
	}
	if _, err := dods.Get(ctx, "dod-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate dod get err = %v, want conflict", err)
	}
	if _, err := memories.Get(ctx, "memory-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate memory get err = %v, want conflict", err)
	}
	if _, err := results.Get(ctx, "result-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate result get err = %v, want conflict", err)
	}
	if _, err := artifacts.Get(ctx, "artifact-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate artifact get err = %v, want conflict", err)
	}
	if _, err := artifacts.ListByRun(ctx, "run-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate artifact run list err = %v, want conflict", err)
	}
	if _, err := artifacts.ListByResult(ctx, "result-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate artifact result list err = %v, want conflict", err)
	}
	if _, err := events.Get(ctx, "event-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate event get err = %v, want conflict", err)
	}
	if _, err := events.List(ctx, workorchquerycontract.EventRecordQuery{RunID: "run-shared"}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate event run list err = %v, want conflict", err)
	}

	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-local-b")
	goal, err := goals.Get(scoped, "goal-shared")
	if err != nil {
		t.Fatalf("scoped goal get: %v", err)
	}
	if goal.WorkspaceID != "ws-local-b" {
		t.Fatalf("scoped goal workspace = %q, want ws-local-b", goal.WorkspaceID)
	}
	eventItems, err := events.List(scoped, workorchquerycontract.EventRecordQuery{RunID: "run-shared"})
	if err != nil {
		t.Fatalf("scoped event list: %v", err)
	}
	if len(eventItems) != 1 || eventItems[0].WorkspaceID != "ws-local-b" {
		t.Fatalf("scoped event list = %#v, want one ws-local-b event", eventItems)
	}
}

func TestEventRecordRepositoryFailsClosedOnCorruptPersistedOccurredAt(t *testing.T) {
	ctx := context.Background()
	repo := NewEventRecordRepository()
	record := &eventstreammodel.EventRecord{
		ID: "event-corrupt-time", Version: "workorch.event", Topic: "/test", OccurredAt: "2026-08-30T00:00:00Z",
		SourceModule: "blocks/test", SubjectType: "test", SubjectID: "corrupt-time", StreamPosition: 1,
	}
	if err := repo.Append(ctx, record); err != nil {
		t.Fatal(err)
	}
	for _, stored := range repo.items {
		if stored != nil && stored.ID == record.ID {
			stored.OccurredAt = "invalid"
		}
	}
	if _, err := repo.Get(ctx, record.ID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Get corrupt OccurredAt error = %v, want conflict", err)
	}
	if _, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("List corrupt OccurredAt error = %v, want conflict", err)
	}
}
