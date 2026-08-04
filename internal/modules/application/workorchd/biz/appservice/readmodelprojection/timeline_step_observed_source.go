package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func timelineStepIDFromObservedEvent(item projectioncontract.ObservedView) (string, bool) {
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
