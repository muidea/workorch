package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
)

func timelineStepIDFromObservedEvent(item presenter.ObservedView) (string, bool) {
	if item.Topic != runevents.TopicStepDispatched && item.Topic != runevents.TopicStepStateChanged && item.SubjectType != "step" {
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
