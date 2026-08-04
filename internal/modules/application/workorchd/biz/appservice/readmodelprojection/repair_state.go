package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRepairLoopRecoveryState(ret *projectioncontract.RepairLoop, recovery projectioncontract.RecoveryPlan, attemptCount int, hasLastFailure bool) {
	if ret == nil {
		return
	}
	switch recovery.Mode {
	case "wait_decision":
		ret.State = "awaiting_decision"
	case "wait_approval":
		ret.State = "awaiting_approval"
	case "none":
		if attemptCount > 0 {
			ret.State = "completed_after_repair"
		}
	case "inspect_failure", "restart":
		ret.State = "terminated"
	default:
		if attemptCount > 0 || hasLastFailure {
			ret.State = "repairing"
		}
	}
}
