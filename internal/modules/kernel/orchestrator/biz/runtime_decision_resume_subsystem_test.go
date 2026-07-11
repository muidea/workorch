package orchestrator

import (
	"context"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestDecisionResumeSubsystemSelectsNewestReplanForFailedStep(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	subsystem := newDecisionResumeSubsystem(ticketRepo, resolutionRepo)
	runID := common.RunID("run-decision-resume")
	failedStepID := common.StepID("run-decision-resume/failed-step")

	tickets := []*decisionmodel.Ticket{
		{
			ID:        "ticket-old-replan",
			RunID:     runID,
			StepID:    failedStepID,
			State:     common.DecisionTicketStateResolved,
			CreatedAt: "2026-06-01T00:00:00Z",
			UpdatedAt: "2026-06-01T00:00:01Z",
		},
		{
			ID:        "ticket-new-replan",
			RunID:     runID,
			StepID:    failedStepID,
			State:     common.DecisionTicketStateResolved,
			CreatedAt: "2026-06-01T00:00:00Z",
			UpdatedAt: "2026-06-01T00:00:02Z",
		},
		{
			ID:        "ticket-newer-non-replan",
			RunID:     runID,
			StepID:    failedStepID,
			State:     common.DecisionTicketStateResolved,
			UpdatedAt: "2026-06-01T00:00:03Z",
		},
	}
	for _, ticket := range tickets {
		if err := ticketRepo.Create(ctx, ticket); err != nil {
			t.Fatalf("create ticket %s: %v", ticket.ID, err)
		}
	}
	resolutions := []*decisionmodel.Resolution{
		{ID: "resolution-old-replan", TicketID: "ticket-old-replan", Type: decisioncommon.ResolutionTypeReplan},
		{ID: "resolution-new-replan", TicketID: "ticket-new-replan", Type: decisioncommon.ResolutionTypeReplan},
		{ID: "resolution-newer-non-replan", TicketID: "ticket-newer-non-replan", Type: decisioncommon.ResolutionTypeProvideMissingInfo},
	}
	for _, resolution := range resolutions {
		if err := resolutionRepo.Create(ctx, resolution); err != nil {
			t.Fatalf("create resolution %s: %v", resolution.ID, err)
		}
	}

	ticket, resolution, err := subsystem.resolvedReplanDecisionForFailedRunStep(ctx, runID, []*runmodel.Step{{
		ID:    failedStepID,
		State: common.StepStateFailed,
	}})
	if err != nil {
		t.Fatalf("resolve replan decision: %v", err)
	}
	if ticket.ID != "ticket-new-replan" || resolution.ID != "resolution-new-replan" {
		t.Fatalf("selected ticket=%s resolution=%s, want newest replan", ticket.ID, resolution.ID)
	}
}
