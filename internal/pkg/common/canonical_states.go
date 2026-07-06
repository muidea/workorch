package common

func CanonicalRunState(state RunState) RunState {
	switch state {
	case RunStatePending:
		return RunStatePending
	case RunStateActive:
		return RunStateActive
	case RunStateBlockedOnDecision:
		return RunStateBlockedOnDecision
	case RunStateBlockedOnApproval:
		return RunStateBlockedOnApproval
	case RunStateCompleted:
		return RunStateCompleted
	case RunStateFailed:
		return RunStateFailed
	case RunStateCancelled:
		return RunStateCancelled
	case RunStateSuperseded:
		return RunStateSuperseded
	default:
		return state
	}
}

func CanonicalStepState(state StepState) StepState {
	switch state {
	case StepStateExecuting:
		return StepStateExecuting
	case StepStateSucceeded:
		return StepStateSucceeded
	case StepStatePending:
		return StepStatePending
	case StepStateFailed:
		return StepStateFailed
	case StepStateSkipped:
		return StepStateSkipped
	case StepStateCancelled:
		return StepStateCancelled
	default:
		return state
	}
}

func CanonicalWorkspaceOwnerState(state WorkspaceState) WorkspaceState {
	switch state {
	case WorkspaceStateBlockedOnApproval:
		return WorkspaceStateBlockedOnApproval
	case WorkspaceStateSuperseded:
		return WorkspaceStateSuperseded
	case WorkspaceStateActive:
		return WorkspaceStateActive
	default:
		return state
	}
}
