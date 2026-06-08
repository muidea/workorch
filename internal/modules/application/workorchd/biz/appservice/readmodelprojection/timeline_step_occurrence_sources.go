package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
)

func timelineStepIDFromEventRecord(item readmodelcore.EventRecord) (string, bool) {
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
