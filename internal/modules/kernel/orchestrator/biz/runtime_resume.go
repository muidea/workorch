package orchestrator

import (
	"context"
	"errors"
	"fmt"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) runningTerminalStepRecoverySpecs(ctx context.Context, run *runModel.Run) ([]stepDispatchSpec, error) {
	if o == nil || run == nil || o.steps == nil {
		return nil, common.ErrConflict
	}
	if specs, ok, err := o.artifactContinuationRecoverySpecs(ctx, run); ok || err != nil {
		return specs, err
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	last := terminalExecutionStep(steps)
	if last == nil {
		return nil, nil
	}
	if last.State != common.StepStateFailed && last.State != common.StepStateDispatched {
		return nil, nil
	}
	return o.nextDispatchSpecsFromStep(ctx, run, last)
}

func (o *RuntimeOrchestrator) failRunningResume(ctx context.Context, runID common.RunID, cause error) error {
	if cause == nil {
		return nil
	}
	current, getErr := o.runs.Get(ctx, runID)
	if getErr != nil {
		return errors.Join(cause, getErr)
	}
	if current.State != common.RunStateRunning {
		return cause
	}
	if err := o.FailRun(ctx, runID, cause.Error()); err != nil {
		return errors.Join(cause, err)
	}
	return nil
}

func (o *RuntimeOrchestrator) runningDecisionRecoveryContext(
	ctx context.Context,
	run *runModel.Run,
) (common.StepID, map[string]any, error) {
	if o == nil || run == nil || o.steps == nil || o.tickets == nil || o.resolutions == nil {
		return "", nil, common.ErrConflict
	}
	if run.BlockingTicketID != "" || run.BlockingApprovalID != "" {
		return "", nil, common.ErrConflict
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return "", nil, err
	}
	last := terminalExecutionStep(steps)
	if last == nil || last.State != common.StepStateFailed {
		return "", nil, common.ErrConflict
	}
	ticket, resolution, err := o.resolvedDecisionForStep(ctx, run.ID, last.ID)
	if errors.Is(err, common.ErrConflict) {
		ticket, resolution, err = o.resolvedReplanDecisionForFailedRunStep(ctx, run.ID, steps)
	}
	if err != nil {
		return "", nil, err
	}
	switch effectiveDecisionResolutionType(ticket, resolution) {
	case decisioncommon.ResolutionTypeProvideMissingInfo:
		return last.ID, decisionResolutionInput(ticket, resolution), nil
	case decisioncommon.ResolutionTypeRejectCurrentPath:
		return "", nil, o.FailRun(ctx, run.ID, fmt.Sprintf("decision ticket %s rejected", ticket.ID))
	case decisioncommon.ResolutionTypeReplan:
		return "", nil, o.replanLoadedRun(ctx, run, ticket, resolution)
	default:
		return "", nil, common.ErrInvalidArgument
	}
}

func (o *RuntimeOrchestrator) resolvedDecisionForStep(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	tickets, err := o.tickets.ListByRunID(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	for _, ticket := range tickets {
		if ticket == nil ||
			ticket.StepID != stepID ||
			ticket.State != common.DecisionTicketStateResolved {
			continue
		}
		resolution, err := o.resolutions.GetByTicketID(ctx, ticket.ID)
		if err != nil {
			return nil, nil, err
		}
		return ticket, resolution, nil
	}
	return nil, nil, common.ErrConflict
}

func (o *RuntimeOrchestrator) resolvedReplanDecisionForFailedRunStep(
	ctx context.Context,
	runID common.RunID,
	steps []*runModel.Step,
) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	if o == nil || runID == "" || o.tickets == nil || o.resolutions == nil {
		return nil, nil, common.ErrConflict
	}
	failedSteps := map[common.StepID]bool{}
	for _, step := range steps {
		if step == nil || step.ID == "" || step.State != common.StepStateFailed {
			continue
		}
		failedSteps[step.ID] = true
	}
	if len(failedSteps) == 0 {
		return nil, nil, common.ErrConflict
	}
	tickets, err := o.tickets.ListByRunID(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	var selectedTicket *decisionmodel.Ticket
	var selectedResolution *decisionmodel.Resolution
	for _, ticket := range tickets {
		if ticket == nil ||
			ticket.RunID != runID ||
			ticket.State != common.DecisionTicketStateResolved ||
			!failedSteps[ticket.StepID] {
			continue
		}
		resolution, err := o.resolutions.GetByTicketID(ctx, ticket.ID)
		if err != nil {
			return nil, nil, err
		}
		if effectiveDecisionResolutionType(ticket, resolution) != decisioncommon.ResolutionTypeReplan {
			continue
		}
		if selectedTicket == nil || resolvedDecisionTicketIsNewer(ticket, selectedTicket) {
			selectedTicket = ticket
			selectedResolution = resolution
		}
	}
	if selectedTicket == nil {
		return nil, nil, common.ErrConflict
	}
	return selectedTicket, selectedResolution, nil
}

func resolvedDecisionTicketIsNewer(candidate *decisionmodel.Ticket, current *decisionmodel.Ticket) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if candidate.UpdatedAt != "" || current.UpdatedAt != "" {
		if candidate.UpdatedAt != current.UpdatedAt {
			return candidate.UpdatedAt > current.UpdatedAt
		}
	}
	if candidate.CreatedAt != "" || current.CreatedAt != "" {
		if candidate.CreatedAt != current.CreatedAt {
			return candidate.CreatedAt > current.CreatedAt
		}
	}
	return candidate.ID > current.ID
}
