package readmodelprojection

import (
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: run_step_governance_blockers.go

func blockingDecisionRunStatusStep(run *runmodel.Run, decisions []*decisionmodel.Ticket, byID map[common.StepID]*runmodel.Step) *runmodel.Step {
	if run == nil || run.BlockingTicketID == "" {
		return nil
	}
	for _, item := range decisions {
		if item != nil && item.ID == run.BlockingTicketID && item.StepID != "" {
			if step := byID[item.StepID]; step != nil {
				return step
			}
		}
	}
	return nil
}

func blockingApprovalRunStatusStep(run *runmodel.Run, approvals []*approvalmodel.Approval, byID map[common.StepID]*runmodel.Step) *runmodel.Step {
	if run == nil || run.BlockingApprovalID == "" {
		return nil
	}
	for _, item := range approvals {
		if item != nil && item.ID == run.BlockingApprovalID && item.StepID != "" {
			if step := byID[item.StepID]; step != nil {
				return step
			}
		}
	}
	return nil
}

// source: run_step_selection.go

func currentRunStatusStep(run *runmodel.Run, steps []*runmodel.Step, checkpoints []*runmodel.RuntimeCheckpoint, decisions []*decisionmodel.Ticket, approvals []*approvalmodel.Approval) *runmodel.Step {
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
	if run != nil && isTerminalRunState(string(run.State)) {
		if step := terminalRunStatusStepWithCheckpoints(steps, checkpoints); step != nil {
			return step
		}
	}
	if checkpoint := latestRuntimeCheckpointModel(checkpoints); checkpoint != nil && checkpoint.CurrentStepID != "" {
		if step := byID[checkpoint.CurrentStepID]; step != nil {
			if runningCompletedStepWouldMasqueradeAsActive(run, step.State) && latestNonTerminalRunStatusStep(steps) == nil {
				return nil
			}
			return step
		}
	}
	if step := latestNonTerminalRunStatusStep(steps); step != nil {
		return step
	}
	if step := terminalRunStatusStep(steps); step != nil {
		if runningCompletedStepWouldMasqueradeAsActive(run, step.State) {
			return nil
		}
		return step
	}
	return lastStep(steps)
}

func runningCompletedStepWouldMasqueradeAsActive(run *runmodel.Run, state common.StepState) bool {
	return run != nil && !isTerminalRunState(string(run.State)) && state == common.StepStateSucceeded
}
