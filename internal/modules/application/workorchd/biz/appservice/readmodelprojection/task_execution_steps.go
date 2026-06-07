package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"

func taskExecutionStepsByRoot(steps []presenter.StepView) map[string]map[string]presenter.StepView {
	ret := map[string]map[string]presenter.StepView{}
	for _, step := range steps {
		nodeID := readModelStringValue(step.Input["graphNodeId"])
		if nodeID == "" {
			continue
		}
		rootStepID := readModelStringValue(step.Input["graphRootStepId"])
		if rootStepID == "" {
			rootStepID = graphRootStepIDFromStepID(step.ID)
		}
		if rootStepID == "" {
			continue
		}
		if ret[rootStepID] == nil {
			ret[rootStepID] = map[string]presenter.StepView{}
		}
		ret[rootStepID][nodeID] = step
	}
	return ret
}
