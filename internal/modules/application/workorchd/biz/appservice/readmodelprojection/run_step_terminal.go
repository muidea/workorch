package readmodelprojection

import (
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func LastStep(steps []*runmodel.Step) *runmodel.Step {
	if len(steps) == 0 {
		return nil
	}
	return steps[len(steps)-1]
}

func LatestNonTerminalRunStatusStep(steps []*runmodel.Step) *runmodel.Step {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step == nil {
			continue
		}
		if step.State == common.StepStateDispatched || step.State == common.StepStatePending {
			return step
		}
	}
	return nil
}

func TerminalRunStatusStep(steps []*runmodel.Step) *runmodel.Step {
	terminals := terminalRunStatusStepCandidates(steps)
	if len(terminals) == 0 {
		return nil
	}
	for _, step := range terminals {
		if step.State == common.StepStateDispatched {
			return step
		}
	}
	return terminals[len(terminals)-1]
}
