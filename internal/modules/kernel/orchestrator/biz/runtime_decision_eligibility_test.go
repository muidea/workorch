package orchestrator

import (
	"context"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDecisionEligibilityOpenTicketWithoutContinuableFactBlocks(t *testing.T) {
	orch := &RuntimeOrchestrator{}
	run := &runmodel.Run{
		ID:               "run-decision-eligibility",
		WorkspaceID:      "ws-decision-eligibility",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-decision-eligibility",
	}
	ticket := &decisionmodel.Ticket{
		ID:          "ticket-decision-eligibility",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		State:       common.DecisionTicketStateOpen,
	}

	got := orch.evaluateDecisionEligibility(context.Background(), run, ticket)
	if got.Action != decisionEligibilityBlocked || got.ClearBlocker || got.SupersedeTicket || got.ResumeAfterStepID != "" || len(got.ResumeInput) != 0 {
		t.Fatalf("decision eligibility = %+v, want blocked without side effects", got)
	}
}
