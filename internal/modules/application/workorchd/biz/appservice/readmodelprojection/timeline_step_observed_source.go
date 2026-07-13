package readmodelprojection

import ()

func timelineStepIDFromObservedEvent(item ObservedView) (string, bool) {
	if item.Topic != "/step/dispatched" && item.Topic != "/step/state/changed" && item.SubjectType != "step" {
		return "", false
	}
	stepID := item.StepID
	if stepID == "" && item.SubjectType == "step" {
		stepID = item.SubjectID
	}
	if stepID == "" {
		return "", false
	}
	return stepID, true
}
