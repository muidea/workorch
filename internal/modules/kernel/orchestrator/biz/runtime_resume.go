package orchestrator

import (
	"context"
	"errors"
	"fmt"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type artifactContinuationRecoverySpecsFunc func(context.Context, *runModel.Run) ([]stepDispatchSpec, bool, error)
type nextDispatchSpecsFromStepFunc func(context.Context, *runModel.Run, *runModel.Step) ([]stepDispatchSpec, error)
type nextDispatchSpecsFunc func(context.Context, *runModel.Run) ([]stepDispatchSpec, error)

func runningTerminalStepRecoverySpecs(
	ctx context.Context,
	steps repository.StepRepository,
	artifactRecovery artifactContinuationRecoverySpecsFunc,
	nextFromStep nextDispatchSpecsFromStepFunc,
	run *runModel.Run,
) ([]stepDispatchSpec, error) {
	if run == nil || steps == nil || artifactRecovery == nil || nextFromStep == nil {
		return nil, common.ErrConflict
	}
	if specs, ok, err := artifactRecovery(ctx, run); ok || err != nil {
		return specs, err
	}
	runSteps, err := steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	last := terminalExecutionStep(runSteps)
	if last == nil {
		return nil, nil
	}
	if last.State != common.StepStateFailed && last.State != common.StepStateDispatched {
		return nil, nil
	}
	return nextFromStep(ctx, run, last)
}

func runningInitialDispatchRecoverySpecs(
	ctx context.Context,
	steps repository.StepRepository,
	next nextDispatchSpecsFunc,
	run *runModel.Run,
) ([]stepDispatchSpec, error) {
	if run == nil || steps == nil || next == nil {
		return nil, common.ErrConflict
	}
	runSteps, err := steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if len(runSteps) != 0 {
		return nil, nil
	}
	return next(ctx, run)
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
	ticket, resolution, err := o.decisionResumeManager().resolvedDecisionForStep(ctx, run.ID, last.ID)
	if errors.Is(err, common.ErrConflict) {
		ticket, resolution, err = o.decisionResumeManager().resolvedReplanDecisionForFailedRunStep(ctx, run.ID, steps)
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
