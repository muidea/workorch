package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func taskExecutionStepsByRoot(steps []readmodelcore.StepView) map[string]map[string]readmodelcore.StepView {
	ret := map[string]map[string]readmodelcore.StepView{}
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
			ret[rootStepID] = map[string]readmodelcore.StepView{}
		}
		ret[rootStepID][nodeID] = step
	}
	return ret
}
