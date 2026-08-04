package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func repairAttemptProjection(steps []readmodelcore.StepView) (count int, lastAttempt int, lastStepID string) {
	for _, step := range steps {
		if !readModelBoolValue(step.Input["runtimeRepairFollowUp"]) {
			continue
		}
		count++
		attempt := readModelIntValue(step.Input["runtimeRepairAttempt"])
		if attempt <= 0 {
			attempt = 1
		}
		if attempt > lastAttempt || (attempt == lastAttempt && step.ID > lastStepID) {
			lastAttempt = attempt
			lastStepID = step.ID
		}
	}
	if count > 0 && lastAttempt == 0 {
		lastAttempt = count
	}
	return count, lastAttempt, lastStepID
}
