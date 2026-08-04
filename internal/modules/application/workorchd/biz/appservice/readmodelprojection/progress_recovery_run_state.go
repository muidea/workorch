package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRunStateRecovery(ret *projectioncontract.RecoveryPlan, snapshot readmodelcore.RunSnapshotState) {
	if ret == nil {
		return
	}
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.Mode = "none"
		ret.NextAction = "inspect_result"
		ret.Reason = "run completed"
	case common.RunStateFailed:
		ret.Mode = "inspect_failure"
		ret.NextAction = "inspect_failure_reason_and_evidence"
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled, common.RunStateSuperseded:
		ret.Mode = "restart"
		ret.NextAction = "start_new_run"
		ret.Reason = "run is terminal but not completed"
	default:
		ret.Mode = "resume"
		ret.NextAction = "resume_run"
		ret.Reason = "run is not terminal"
	}
}
