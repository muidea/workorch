package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func repairAttemptProjection(steps []projectioncontract.StepView) (count int, lastAttempt int, lastStepID string) {
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
