package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func LatestNonTerminalSnapshotStep(steps []projectioncontract.StepView) *projectioncontract.StepView {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		if steps[idx].State == string(common.StepStateExecuting) || steps[idx].State == string(common.StepStatePending) {
			return &steps[idx]
		}
	}
	return nil
}

func TerminalSnapshotStep(steps []projectioncontract.StepView) *projectioncontract.StepView {
	terminals := terminalSnapshotStepCandidates(steps)
	if len(terminals) == 0 {
		return nil
	}
	for _, step := range terminals {
		if step.State == string(common.StepStateExecuting) {
			return step
		}
	}
	return terminals[len(terminals)-1]
}
