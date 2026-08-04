package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func isStepRuntimeCheckpoint(checkpoint projectioncontract.RuntimeCheckpoint) bool {
	if checkpoint.CurrentStepID == "" || len(checkpoint.Data) == 0 {
		return false
	}
	if readModelStringValue(checkpoint.Data["stepState"]) != "" {
		return true
	}
	eventName := readModelStringValue(checkpoint.Data["event"])
	return eventName == "step_dispatched" || eventName == "step_completed" || eventName == "step_failed"
}
