package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/common"

// WD5 mechanical consolidation: same import contract; source list follows.

// source: memory_recall_ids.go

func memoryIDsToReadModelStrings(input []common.MemoryID) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		ret = append(ret, string(item))
	}
	return ret
}

// source: outcome_state.go

func isTerminalProjectionState(state string) bool {
	switch common.WorkspaceState(state) {
	case common.WorkspaceStateSuperseded:
		return true
	default:
		return false
	}
}

func isTerminalRunState(state string) bool {
	switch common.RunState(state) {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func isTerminalProjectionRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}
