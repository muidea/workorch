package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/common"

func WorkspaceProjectionState(runState common.RunState) common.WorkspaceState {
	switch runState {
	case common.RunStateBlockedOnDecision:
		return common.WorkspaceStateActive
	case common.RunStateBlockedOnApproval:
		return common.WorkspaceStateBlockedOnApproval
	case common.RunStateSuperseded:
		return common.WorkspaceStateSuperseded
	case common.RunStatePending,
		common.RunStateActive,
		common.RunStateCompleted,
		common.RunStateFailed,
		common.RunStateCancelled:
		return common.WorkspaceStateActive
	default:
		return common.WorkspaceStateActive
	}
}

func IsTerminalProjectionRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}
