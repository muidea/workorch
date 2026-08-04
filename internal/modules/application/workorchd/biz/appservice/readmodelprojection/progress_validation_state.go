package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ValidationProgressState(snapshot readmodelcore.RunSnapshotState) string {
	if len(snapshot.ValidationResults) == 0 {
		if snapshot.Run.State == string(common.RunStateActive) {
			return "running"
		}
		return "pending"
	}
	if HasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	for _, item := range snapshot.ValidationResults {
		if item.State == string(common.ValidationResultStateFailed) {
			return "failed"
		}
	}
	return "completed"
}
