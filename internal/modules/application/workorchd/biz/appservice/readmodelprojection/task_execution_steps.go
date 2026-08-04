package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func taskExecutionStepsByRoot(steps []projectioncontract.StepView) map[string]map[string]projectioncontract.StepView {
	ret := map[string]map[string]projectioncontract.StepView{}
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
			ret[rootStepID] = map[string]projectioncontract.StepView{}
		}
		ret[rootStepID][nodeID] = step
	}
	return ret
}
