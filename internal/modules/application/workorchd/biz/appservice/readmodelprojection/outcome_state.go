package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/common"

func IsTerminalProjectionState(state string) bool {
	switch common.WorkspaceState(state) {
	case common.WorkspaceStateCompleted, common.WorkspaceStateFailed, common.WorkspaceStateArchived:
		return true
	default:
		return false
	}
}

func IsTerminalRunState(state string) bool {
	switch common.RunState(state) {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}
