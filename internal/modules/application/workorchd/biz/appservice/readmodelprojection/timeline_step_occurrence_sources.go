package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func timelineStepIDFromEventRecord(item readmodelcore.EventRecord) (string, bool) {
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
