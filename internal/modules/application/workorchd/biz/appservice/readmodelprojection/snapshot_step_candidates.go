package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func LatestNonTerminalSnapshotStep(steps []readmodelcore.StepView) *readmodelcore.StepView {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		if steps[idx].State == string(common.StepStateExecuting) || steps[idx].State == string(common.StepStatePending) {
			return &steps[idx]
		}
	}
	return nil
}

func TerminalSnapshotStep(steps []readmodelcore.StepView) *readmodelcore.StepView {
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
