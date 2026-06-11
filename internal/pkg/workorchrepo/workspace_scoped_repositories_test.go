package workorchrepo

import (
	"context"
	"errors"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceScopedGovernanceGetRequiresUniqueUnscopedID(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	for _, workspaceID := range []common.WorkspaceID{"ws-governance-a", "ws-governance-b"} {
		if err := repos.Runs.Create(ctx, &runmodel.Run{
			ID:          "run-shared-governance",
			WorkspaceID: workspaceID,
			State:       common.RunStateBlockedOnDecision,
		}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
		if err := repos.Tickets.Create(ctx, &decisionmodel.Ticket{
			ID:          "ticket-shared-governance",
			WorkspaceID: workspaceID,
			RunID:       "run-shared-governance",
			State:       common.DecisionTicketStateOpen,
			Reason:      "scope regression",
		}); err != nil {
			t.Fatalf("create ticket %s: %v", workspaceID, err)
		}
		if err := repos.Approvals.Create(ctx, &approvalmodel.Approval{
			ID:          "approval-shared-governance",
			WorkspaceID: workspaceID,
			RunID:       "run-shared-governance",
			State:       common.ApprovalStatePending,
			Reason:      "scope regression",
			Action:      "approve scoped action",
		}); err != nil {
			t.Fatalf("create approval %s: %v", workspaceID, err)
		}
	}

	if _, err := repos.Tickets.Get(ctx, "ticket-shared-governance"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate ticket get err = %v, want conflict", err)
	}
	if _, err := repos.Approvals.Get(ctx, "approval-shared-governance"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate approval get err = %v, want conflict", err)
	}

	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-governance-a")
	ticket, err := repos.Tickets.Get(scoped, "ticket-shared-governance")
	if err != nil {
		t.Fatalf("scoped ticket get: %v", err)
	}
	if ticket.WorkspaceID != "ws-governance-a" {
		t.Fatalf("scoped ticket workspace = %q, want ws-governance-a", ticket.WorkspaceID)
	}
	approval, err := repos.Approvals.Get(scoped, "approval-shared-governance")
	if err != nil {
		t.Fatalf("scoped approval get: %v", err)
	}
	if approval.WorkspaceID != "ws-governance-a" {
		t.Fatalf("scoped approval workspace = %q, want ws-governance-a", approval.WorkspaceID)
	}
}

func TestWorkspaceScopedDefinitionGetRequiresUniqueUnscopedID(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	for _, workspaceID := range []common.WorkspaceID{"ws-definition-a", "ws-definition-b"} {
		if err := repos.Goals.Create(ctx, &workspacemodel.Goal{
			ID:          "goal-shared-definition",
			WorkspaceID: workspaceID,
			Text:        "scoped goal",
		}); err != nil {
			t.Fatalf("create goal %s: %v", workspaceID, err)
		}
		if err := repos.DoDs.Create(ctx, &workspacemodel.DoD{
			ID:          "dod-shared-definition",
			WorkspaceID: workspaceID,
			Items:       []string{"scoped dod"},
			State:       "ready",
		}); err != nil {
			t.Fatalf("create dod %s: %v", workspaceID, err)
		}
	}

	if _, err := repos.Goals.Get(ctx, "goal-shared-definition"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate goal get err = %v, want conflict", err)
	}
	if _, err := repos.DoDs.Get(ctx, "dod-shared-definition"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate dod get err = %v, want conflict", err)
	}

	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-definition-b")
	goal, err := repos.Goals.Get(scoped, "goal-shared-definition")
	if err != nil {
		t.Fatalf("scoped goal get: %v", err)
	}
	if goal.WorkspaceID != "ws-definition-b" {
		t.Fatalf("scoped goal workspace = %q, want ws-definition-b", goal.WorkspaceID)
	}
	dod, err := repos.DoDs.Get(scoped, "dod-shared-definition")
	if err != nil {
		t.Fatalf("scoped dod get: %v", err)
	}
	if dod.WorkspaceID != "ws-definition-b" {
		t.Fatalf("scoped dod workspace = %q, want ws-definition-b", dod.WorkspaceID)
	}
}

func TestWorkspaceScopedRunLocalGetRequiresUniqueUnscopedID(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	for _, workspaceID := range []common.WorkspaceID{"ws-runlocal-a", "ws-runlocal-b"} {
		scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := repos.Runs.Create(scoped, &runmodel.Run{
			ID:          "run-shared-local",
			WorkspaceID: workspaceID,
			State:       common.RunStateRunning,
		}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
		if err := repos.Steps.Create(scoped, &runmodel.Step{
			ID:    "run-shared-local/step-001",
			RunID: "run-shared-local",
			Kind:  "validation",
			State: common.StepStateCompleted,
		}); err != nil {
			t.Fatalf("create step %s: %v", workspaceID, err)
		}
	}

	if _, err := repos.Runs.Get(ctx, "run-shared-local"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate run get err = %v, want conflict", err)
	}
	if _, err := repos.Steps.Get(ctx, "run-shared-local/step-001"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate step get err = %v, want conflict", err)
	}
	if _, err := repos.Steps.ListByRun(ctx, "run-shared-local"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate step list err = %v, want conflict", err)
	}

	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-runlocal-a")
	run, err := repos.Runs.Get(scoped, "run-shared-local")
	if err != nil {
		t.Fatalf("scoped run get: %v", err)
	}
	if run.WorkspaceID != "ws-runlocal-a" {
		t.Fatalf("scoped run workspace = %q, want ws-runlocal-a", run.WorkspaceID)
	}
	step, err := repos.Steps.Get(scoped, "run-shared-local/step-001")
	if err != nil {
		t.Fatalf("scoped step get: %v", err)
	}
	if step.RunID != "run-shared-local" {
		t.Fatalf("scoped step run = %q, want run-shared-local", step.RunID)
	}
	steps, err := repos.Steps.ListByRun(scoped, "run-shared-local")
	if err != nil {
		t.Fatalf("scoped step list: %v", err)
	}
	if len(steps) != 1 || steps[0].ID != "run-shared-local/step-001" {
		t.Fatalf("scoped step list = %#v, want one scoped step", steps)
	}
}
