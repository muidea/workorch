package activationports

import (
	"context"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

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
		Payload:  workorcheventpayload.New(workorcheventpayload.StringField("choice", "ok")),
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
