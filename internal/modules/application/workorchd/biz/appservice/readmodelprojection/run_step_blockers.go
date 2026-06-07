package readmodelprojection

import (
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func runStatusStepIndex(steps []*runmodel.Step) map[common.StepID]*runmodel.Step {
	byID := make(map[common.StepID]*runmodel.Step, len(steps))
	for _, step := range steps {
		if step == nil || step.ID == "" {
			continue
		}
		byID[step.ID] = step
	}
	return byID
}
