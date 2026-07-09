package orchestrator

import (
	"context"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type decisionEligibilityAction string

const (
	decisionEligibilityBlocked    decisionEligibilityAction = "blocked"
	decisionEligibilityAutoResume decisionEligibilityAction = "auto_resume"
	decisionEligibilityAutoRepair decisionEligibilityAction = "auto_repair"
)

type decisionEligibilityResult struct {
	Action            decisionEligibilityAction
	ResumeAfterStepID common.StepID
	ResumeInput       map[string]any
	SupersedeTicket   bool
	ClearBlocker      bool
	Reason            string
}

func (o *RuntimeOrchestrator) evaluateDecisionEligibility(ctx context.Context, r *runModel.Run, ticket *decisionmodel.Ticket) decisionEligibilityResult {
	if o == nil || r == nil || ticket == nil || ticket.State == common.DecisionTicketStateResolved {
		return decisionEligibilityResult{Action: decisionEligibilityBlocked, Reason: "decision is not eligible for unresolved auto continuation"}
	}
	if runtimeRepairExhaustionBlockerHasSuccessEvidence(ctx, o.steps, ticket) {
		return decisionEligibilityResult{
			Action:       decisionEligibilityAutoResume,
			ClearBlocker: true,
			Reason:       "repair exhaustion blocker already has success evidence",
		}
	}
	policy := o.RuntimePolicy()
	if duplicateRuntimeFailureBlockerCanAutoResume(ctx, o.steps, o.repairManager(), policy, r, ticket) {
		return decisionEligibilityResult{
			Action:            decisionEligibilityAutoResume,
			ResumeAfterStepID: ticket.StepID,
			ClearBlocker:      true,
			Reason:            "duplicate runtime failure can resume within repair budget",
		}
	}
	if runtimeRepairExhaustionBlockerCanAutoResume(ctx, o.steps, o.repairManager(), policy, r, ticket) {
		return decisionEligibilityResult{
			Action:            decisionEligibilityAutoResume,
			ResumeAfterStepID: ticket.StepID,
			SupersedeTicket:   true,
			ClearBlocker:      true,
			Reason:            "repair exhaustion blocker can continue within scoped budget",
		}
	}
	if knownReadableDecisionBlockerCanAutoRepair(ctx, o.steps, o.workspaces, r, ticket) {
		return decisionEligibilityResult{
			Action:            decisionEligibilityAutoRepair,
			ResumeAfterStepID: ticket.StepID,
			ResumeInput:       map[string]any{"knownReadableDecisionRepair": true},
			SupersedeTicket:   true,
			ClearBlocker:      true,
			Reason:            "decision blocker can be repaired from known readable workspace resources",
		}
	}
	return decisionEligibilityResult{Action: decisionEligibilityBlocked, Reason: "open decision requires external resolution"}
}
