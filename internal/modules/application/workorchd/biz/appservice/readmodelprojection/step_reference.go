package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func PreviousStepID(step *runmodel.Step) common.StepID {
	if step == nil || step.Input == nil {
		return ""
	}
	for _, key := range []string{"previousStepId", "repairOfStepId", "failedStepId"} {
		value, _ := step.Input[key].(string)
		value = strings.TrimSpace(value)
		if value != "" && common.StepID(value) != step.ID {
			return common.StepID(value)
		}
	}
	return ""
}

func PreviousStepIDSnapshot(step presenter.StepView) string {
	if step.Input == nil {
		return ""
	}
	value, _ := step.Input["previousStepId"].(string)
	return strings.TrimSpace(value)
}
