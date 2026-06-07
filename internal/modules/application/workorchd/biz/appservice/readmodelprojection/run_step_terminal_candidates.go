package readmodelprojection

import (
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func terminalRunStatusStepCandidates(steps []*runmodel.Step) []*runmodel.Step {
	referenced := referencedPreviousRunStatusStepIDs(steps)
	var terminals []*runmodel.Step
	for _, step := range steps {
		if step == nil || referenced[step.ID] {
			continue
		}
		terminals = append(terminals, step)
	}
	return terminals
}

func referencedPreviousRunStatusStepIDs(steps []*runmodel.Step) map[common.StepID]bool {
	referenced := map[common.StepID]bool{}
	for _, step := range steps {
		if previous := PreviousStepID(step); previous != "" {
			referenced[previous] = true
		}
	}
	return referenced
}
