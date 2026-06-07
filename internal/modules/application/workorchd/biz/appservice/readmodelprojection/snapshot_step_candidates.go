package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	"github.com/muidea/workorch/internal/pkg/common"
)

func LatestNonTerminalSnapshotStep(steps []presenter.StepView) *presenter.StepView {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		if steps[idx].State == string(common.StepStateDispatched) || steps[idx].State == string(common.StepStatePending) {
			return &steps[idx]
		}
	}
	return nil
}

func TerminalSnapshotStep(steps []presenter.StepView) *presenter.StepView {
	terminals := terminalSnapshotStepCandidates(steps)
	if len(terminals) == 0 {
		return nil
	}
	for _, step := range terminals {
		if step.State == string(common.StepStateDispatched) {
			return step
		}
	}
	return terminals[len(terminals)-1]
}
