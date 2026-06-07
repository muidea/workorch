package activationcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func IsTerminalRunState(state common.RunState) bool {
	return runtimecore.IsTerminalRunState(state)
}

func IsObservableRunState(state common.RunState) bool {
	switch state {
	case common.RunStateRunning,
		common.RunStateValidating,
		common.RunStateBlockedOnDecision,
		common.RunStateBlockedOnApproval,
		common.RunStateCompleted,
		common.RunStateFailed,
		common.RunStateCancelled,
		common.RunStateSuperseded:
		return true
	default:
		return false
	}
}
