package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	"github.com/muidea/workorch/internal/pkg/events"
)

func timelineStepIDFromObservedEvent(item presenter.ObservedView) (string, bool) {
	if item.Topic != events.TopicStepDispatched && item.Topic != events.TopicStepStateChanged && item.SubjectType != "step" {
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
