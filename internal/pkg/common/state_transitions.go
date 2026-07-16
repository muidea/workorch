package common

func RunStateIsTerminal(state RunState) bool {
	switch state {
	case RunStateCompleted, RunStateFailed, RunStateCancelled, RunStateSuperseded:
		return true
	default:
		return false
	}
}

func RunStateCanTransition(from RunState, to RunState) bool {
	if from == to {
		return true
	}
	switch from {
	case RunStatePending:
		switch to {
		case RunStateActive, RunStateBlockedOnDecision, RunStateBlockedOnApproval, RunStateCancelled, RunStateFailed, RunStateSuperseded:
			return true
		}
	case RunStateActive:
		switch to {
		case RunStateBlockedOnDecision, RunStateBlockedOnApproval, RunStateCompleted, RunStateFailed, RunStateCancelled, RunStateSuperseded:
			return true
		}
	case RunStateBlockedOnDecision, RunStateBlockedOnApproval:
		switch to {
		case RunStateActive, RunStateFailed, RunStateCancelled, RunStateSuperseded:
			return true
		}
	}
	return false
}

func StepStateIsTerminal(state StepState) bool {
	switch state {
	case StepStateSucceeded, StepStateFailed, StepStateSkipped, StepStateCancelled:
		return true
	default:
		return false
	}
}

func StepStateCanTransition(from StepState, to StepState) bool {
	if from == to {
		return true
	}
	switch from {
	case StepStatePending:
		switch to {
		case StepStateExecuting, StepStateSkipped, StepStateCancelled, StepStateFailed:
			return true
		}
	case StepStateExecuting:
		switch to {
		case StepStateSucceeded, StepStateFailed, StepStateCancelled:
			return true
		}
	}
	return false
}
