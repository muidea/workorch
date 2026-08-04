package readmodelprojection

import (
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
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

func PreviousStepIDSnapshot(step projectioncontract.StepView) string {
	if step.Input == nil {
		return ""
	}
	value, _ := step.Input["previousStepId"].(string)
	return strings.TrimSpace(value)
}
