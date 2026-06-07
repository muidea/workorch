package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/common"

func WorkspaceProjectionState(runState common.RunState) common.WorkspaceState {
	switch runState {
	case common.RunStateCreated:
		return common.WorkspaceStateReady
	case common.RunStateRunning:
		return common.WorkspaceStateRunning
	case common.RunStateBlockedOnDecision:
		return common.WorkspaceStateWaitingDecision
	case common.RunStateBlockedOnApproval:
		return common.WorkspaceStateBlocked
	case common.RunStateValidating:
		return common.WorkspaceStateValidating
	case common.RunStateCompleted:
		return common.WorkspaceStateCompleted
	case common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return common.WorkspaceStateFailed
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
