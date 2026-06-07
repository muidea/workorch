package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func StepProgressState(snapshot readmodelcore.RunSnapshot) string {
	if len(snapshot.Steps) == 0 {
		return "pending"
	}
	if HasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	for _, item := range snapshot.Steps {
		if item.State == string(common.StepStateFailed) {
			return "failed"
		}
	}
	if MapRuntimeProgress(snapshot).StepCompleted == len(snapshot.Steps) {
		return "completed"
	}
	return "running"
}
