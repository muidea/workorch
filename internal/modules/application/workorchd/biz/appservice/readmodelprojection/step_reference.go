package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func PreviousStepID(step *runmodel.Step) common.StepID {
	if step == nil {
		return ""
	}
	for _, key := range []string{"previousStepId", "repairOfStepId", "failedStepId"} {
		value, _ := step.Input.String(key)
		value = strings.TrimSpace(value)
		if value != "" && common.StepID(value) != step.ID {
			return common.StepID(value)
		}
	}
	return ""
}

func PreviousStepIDSnapshot(step readmodelcore.StepView) string {
	if step.Input == nil {
		return ""
	}
	value, _ := step.Input["previousStepId"].(string)
	return strings.TrimSpace(value)
}
