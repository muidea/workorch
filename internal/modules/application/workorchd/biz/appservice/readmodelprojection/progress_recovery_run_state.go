package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func applyRunStateRecovery(ret *readmodelcore.RecoveryPlan, snapshot readmodelcore.RunSnapshot) {
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
	case common.RunStateValidating:
		ret.Mode = "resume"
		ret.NextAction = "resume_validation"
		ret.Reason = "run is validating"
	default:
		ret.Mode = "resume"
		ret.NextAction = "resume_run"
		ret.Reason = "run is not terminal"
	}
}
