package readmodelprojection

import (
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CurrentRunStatusStep(run *runmodel.Run, steps []*runmodel.Step, checkpoints []*runmodel.RuntimeCheckpoint, decisions []*decisionmodel.Ticket, approvals []*approvalmodel.Approval) *runmodel.Step {
	if len(steps) == 0 {
		return nil
	}
	byID := runStatusStepIndex(steps)
	if step := blockingDecisionRunStatusStep(run, decisions, byID); step != nil {
		return step
	}
	if step := blockingApprovalRunStatusStep(run, approvals, byID); step != nil {
		return step
	}
	if run != nil && IsTerminalRunState(string(run.State)) {
		if step := TerminalRunStatusStepWithCheckpoints(steps, checkpoints); step != nil {
			return step
		}
	}
	if checkpoint := latestRuntimeCheckpointModel(checkpoints); checkpoint != nil && checkpoint.CurrentStepID != "" {
		if step := byID[checkpoint.CurrentStepID]; step != nil {
			if runningCompletedStepWouldMasqueradeAsActive(run, step.State) && LatestNonTerminalRunStatusStep(steps) == nil {
				return nil
			}
			return step
		}
	}
	if step := LatestNonTerminalRunStatusStep(steps); step != nil {
		return step
	}
	if step := TerminalRunStatusStep(steps); step != nil {
		if runningCompletedStepWouldMasqueradeAsActive(run, step.State) {
			return nil
		}
		return step
	}
	return LastStep(steps)
}

func runningCompletedStepWouldMasqueradeAsActive(run *runmodel.Run, state common.StepState) bool {
	return run != nil && !IsTerminalRunState(string(run.State)) && state == common.StepStateCompleted
}
