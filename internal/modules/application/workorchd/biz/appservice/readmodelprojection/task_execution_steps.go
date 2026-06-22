package readmodelprojection

func taskExecutionStepsByRoot(steps []StepView) map[string]map[string]StepView {
	ret := map[string]map[string]StepView{}
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
			ret[rootStepID] = map[string]StepView{}
		}
		ret[rootStepID][nodeID] = step
	}
	return ret
}
