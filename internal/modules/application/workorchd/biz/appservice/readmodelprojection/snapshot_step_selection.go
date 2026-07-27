package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CurrentRunSnapshotStepForRun(run *runmodel.Run, steps []StepView, decisionStep string, approvalStep string, checkpoints []readmodelcore.RuntimeCheckpoint) *StepView {
	if run != nil && IsTerminalRunState(string(run.State)) {
		if step := currentRunSnapshotTerminalStep(steps, checkpoints); step != nil {
			return step
		}
	}
	return CurrentRunSnapshotStep(steps, decisionStep, approvalStep, checkpoints)
}

func CurrentRunSnapshotStep(steps []StepView, decisionStep string, approvalStep string, checkpoints []readmodelcore.RuntimeCheckpoint) *StepView {
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
			if runningCompletedSnapshotStepWouldMasqueradeAsActive(step) && LatestNonTerminalSnapshotStep(steps) == nil {
				return nil
			}
			return step
		}
	}
	if len(steps) == 0 {
		return nil
	}
	if step := LatestNonTerminalSnapshotStep(steps); step != nil {
		return step
	}
	if step := TerminalSnapshotStep(steps); step != nil {
		if runningCompletedSnapshotStepWouldMasqueradeAsActive(step) {
			return nil
		}
		return step
	}
	return &steps[len(steps)-1]
}

func runningCompletedSnapshotStepWouldMasqueradeAsActive(step *StepView) bool {
	return step != nil && step.State == string(common.StepStateSucceeded)
}

func currentRunSnapshotTerminalStep(steps []StepView, checkpoints []readmodelcore.RuntimeCheckpoint) *StepView {
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

func latestFailedSnapshotStepFromCheckpoints(terminals []*StepView, checkpoints []readmodelcore.RuntimeCheckpoint) *StepView {
	if len(terminals) == 0 || len(checkpoints) == 0 {
		return nil
	}
	terminalByID := map[string]*StepView{}
	for _, step := range terminals {
		if step == nil || step.ID == "" {
			continue
		}
		terminalByID[step.ID] = step
	}
	var selected *StepView
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
