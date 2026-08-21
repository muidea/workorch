package run

import "github.com/muidea/workorch/internal/pkg/common"

// runStateCanTransition is the Run Block's SM-1 persistence invariant. It is
// intentionally private: no shared package or other owner may choose Run
// lifecycle transitions.
func runStateCanTransition(from, to common.RunState) bool {
	if !validRunLifecycleState(from) || !validRunLifecycleState(to) {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case common.RunStatePending:
		return to == common.RunStateActive || to == common.RunStateBlockedOnDecision || to == common.RunStateBlockedOnApproval || to == common.RunStateCancelled || to == common.RunStateFailed || to == common.RunStateSuperseded
	case common.RunStateActive:
		return to == common.RunStateBlockedOnDecision || to == common.RunStateBlockedOnApproval || to == common.RunStateCompleted || to == common.RunStateFailed || to == common.RunStateCancelled || to == common.RunStateSuperseded
	case common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		return to == common.RunStateActive || to == common.RunStateFailed || to == common.RunStateCancelled || to == common.RunStateSuperseded
	default:
		return false
	}
}

func validRunLifecycleState(state common.RunState) bool {
	switch state {
	case common.RunStatePending,
		common.RunStateActive,
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
