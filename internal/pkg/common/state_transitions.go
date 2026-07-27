package common

func RunStateIsTerminal(state RunState) bool {
	switch state {
	case RunStateCompleted, RunStateFailed, RunStateCancelled, RunStateSuperseded:
		return true
	default:
		return false
	}
}

func StepStateIsTerminal(state StepState) bool {
	switch state {
	case StepStateSucceeded, StepStateFailed, StepStateSkipped, StepStateCancelled:
		return true
	default:
		return false
	}
}
