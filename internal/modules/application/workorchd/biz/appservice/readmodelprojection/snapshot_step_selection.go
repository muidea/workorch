package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func currentRunSnapshotStepForRun(run *runmodel.Run, steps []projectioncontract.StepView, decisionStep string, approvalStep string, checkpoints []projectioncontract.RuntimeCheckpoint) *projectioncontract.StepView {
	if run != nil && isTerminalRunState(string(run.State)) {
		if step := currentRunSnapshotTerminalStep(steps, checkpoints); step != nil {
			return step
		}
	}
	return currentRunSnapshotStep(steps, decisionStep, approvalStep, checkpoints)
}

func currentRunSnapshotStep(steps []projectioncontract.StepView, decisionStep string, approvalStep string, checkpoints []projectioncontract.RuntimeCheckpoint) *projectioncontract.StepView {
	if decisionStep != "" {
		if step := stepByID(steps, decisionStep); step != nil {
			return step
		}
	}
	if approvalStep != "" {
		if step := stepByID(steps, approvalStep); step != nil {
			return step
		}
	}
	if checkpoint := latestRuntimeCheckpoint(checkpoints); checkpoint != nil {
		if step := stepByID(steps, checkpoint.CurrentStepID); step != nil {
			if runningCompletedSnapshotStepWouldMasqueradeAsActive(step) && latestNonTerminalSnapshotStep(steps) == nil {
				return nil
			}
			return step
		}
	}
	if len(steps) == 0 {
		return nil
	}
	if step := latestNonTerminalSnapshotStep(steps); step != nil {
		return step
	}
	if step := terminalSnapshotStep(steps); step != nil {
		if runningCompletedSnapshotStepWouldMasqueradeAsActive(step) {
			return nil
		}
		return step
	}
	return &steps[len(steps)-1]
}

func runningCompletedSnapshotStepWouldMasqueradeAsActive(step *projectioncontract.StepView) bool {
	return step != nil && step.State == string(common.StepStateSucceeded)
}

func currentRunSnapshotTerminalStep(steps []projectioncontract.StepView, checkpoints []projectioncontract.RuntimeCheckpoint) *projectioncontract.StepView {
	terminals := terminalSnapshotStepCandidates(steps)
	if len(terminals) == 0 {
		return nil
	}
	if step := latestFailedSnapshotStepFromCheckpoints(terminals, checkpoints); step != nil {
		return step
	}
	for idx := len(terminals) - 1; idx >= 0; idx-- {
		if terminals[idx].State == string(common.StepStateFailed) {
			return terminals[idx]
		}
	}
	return terminals[len(terminals)-1]
}

func latestFailedSnapshotStepFromCheckpoints(terminals []*projectioncontract.StepView, checkpoints []projectioncontract.RuntimeCheckpoint) *projectioncontract.StepView {
	if len(terminals) == 0 || len(checkpoints) == 0 {
		return nil
	}
	terminalByID := map[string]*projectioncontract.StepView{}
	for _, step := range terminals {
		if step == nil || step.ID == "" {
			continue
		}
		terminalByID[step.ID] = step
	}
	var selected *projectioncontract.StepView
	selectedAt := ""
	for _, checkpoint := range checkpoints {
		if checkpoint.CurrentStepID == "" {
			continue
		}
		step := terminalByID[checkpoint.CurrentStepID]
		if step == nil || step.State != string(common.StepStateFailed) {
			continue
		}
		if selected == nil || checkpoint.OccurredAt > selectedAt {
			selected = step
			selectedAt = checkpoint.OccurredAt
		}
	}
	return selected
}
