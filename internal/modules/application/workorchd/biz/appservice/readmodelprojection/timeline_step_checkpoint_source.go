package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func isStepRuntimeCheckpoint(checkpoint readmodelcore.RuntimeCheckpoint) bool {
	if checkpoint.CurrentStepID == "" || len(checkpoint.Data) == 0 {
		return false
	}
	if readModelStringValue(checkpoint.Data["stepState"]) != "" {
		return true
	}
	eventName := readModelStringValue(checkpoint.Data["event"])
	return eventName == "step_dispatched" || eventName == "step_completed" || eventName == "step_failed"
}
