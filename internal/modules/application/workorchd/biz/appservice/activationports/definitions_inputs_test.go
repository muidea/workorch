package activationports

import (
	"context"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadWorkspaceRuntimeActivationInputsReadsDefinitionsWithExplicitScope(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	defer runtime.Shutdown(ctx)
	exports := testRuntimeExports(runtime)
	workspaceID := common.WorkspaceID("ws-activation-input-scope")
	goalID := common.GoalID("goal-activation-input-scope")
	dodID := common.DoDID("dod-activation-input-scope")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "scoped workspace", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Goals.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "scoped goal"}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"scoped dod"}, State: common.DoDStateReady}); err != nil {
		t.Fatal(err)
	}

	inputs, err := ReadWorkspaceRuntimeActivationInputs(ctx, exports.Infrastructure.CommandHub, workspaceID, goalID, dodID)
	if err != nil {
		t.Fatalf("ReadWorkspaceRuntimeActivationInputs err = %v", err)
	}
	if inputs.Workspace.ID != workspaceID || inputs.Goal.WorkspaceID != workspaceID || inputs.DoD.WorkspaceID != workspaceID {
		t.Fatalf("inputs = %+v, want workspace scoped inputs", inputs)
	}
}

func TestWorkspaceGovernanceHistoryReadsRunAndResolutionWithWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	defer runtime.Shutdown(ctx)
	exports := testRuntimeExports(runtime)
	workspaceID := common.WorkspaceID("ws-governance-history-scope")
	runID := common.RunID("run-001")
	ticketID := common.TicketID("run-001-step-1-decision")
	goalID := common.GoalID("goal-history")
	dodID := common.DoDID("dod-history")

	if err := exports.Repositories.Tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-001-step-1"),
		State:       common.DecisionTicketStateResolved,
		Reason:      "history",
		Question:    "question",
	}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		GoalID:      goalID,
		DoDID:       dodID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := exports.Repositories.Resolutions.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-1"),
		TicketID: ticketID,
		Type:     "selected",
		Payload:  map[string]any{"choice": "ok"},
	}); err != nil {
		t.Fatal(err)
	}

	history := WorkspaceGovernanceHistory(ctx, exports.Infrastructure.CommandHub, workspaceID, goalID, dodID, 1)
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1: %+v", len(history), history)
	}
	if history[0].TicketID != ticketID || history[0].RunID != runID {
		t.Fatalf("history = %+v, want scoped ticket/run", history[0])
	}
}
