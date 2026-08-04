package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type taskPlanFields struct {
	anchorStepID string
	stage        string
	state        string
	occurredAt   string
}

func taskPlanProjectionFields(decomposition projectioncontract.TaskDecomposition, step projectioncontract.StepView, hasStep bool, occurrences map[string]string) taskPlanFields {
	ret := taskPlanFields{
		anchorStepID: strings.TrimSpace(decomposition.RootStepID),
		state:        taskPlanStateFromDecomposition(decomposition.State),
		occurredAt:   strings.TrimSpace(occurrences[decomposition.RootStepID]),
	}
	if !hasStep {
		return ret
	}
	if root := strings.TrimSpace(readModelStringValue(step.Input["graphRootStepId"])); root != "" {
		ret.anchorStepID = root
	}
	ret.stage = strings.TrimSpace(readModelStringValue(step.Output["stage"]))
	if ret.stage == "" {
		ret.stage = strings.TrimSpace(readModelStringValue(step.Input["stage"]))
	}
	ret.state = taskPlanStateFromStep(step.State)
	return ret
}
