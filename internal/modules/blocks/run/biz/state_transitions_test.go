package run

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunStateTransitionsRemainOwnedByRunBlock(t *testing.T) {
	states := []common.RunState{
		common.RunStatePending,
		common.RunStateActive,
		common.RunStateBlockedOnDecision,
		common.RunStateBlockedOnApproval,
		common.RunStateCompleted,
		common.RunStateFailed,
		common.RunStateCancelled,
		common.RunStateSuperseded,
		"unknown",
	}
	allowed := map[common.RunState]map[common.RunState]bool{
		common.RunStatePending: {
			common.RunStatePending: true, common.RunStateActive: true,
			common.RunStateBlockedOnDecision: true, common.RunStateBlockedOnApproval: true,
			common.RunStateFailed: true, common.RunStateCancelled: true, common.RunStateSuperseded: true,
		},
		common.RunStateActive: {
			common.RunStateActive:            true,
			common.RunStateBlockedOnDecision: true, common.RunStateBlockedOnApproval: true,
			common.RunStateCompleted: true, common.RunStateFailed: true,
			common.RunStateCancelled: true, common.RunStateSuperseded: true,
		},
		common.RunStateBlockedOnDecision: {
			common.RunStateBlockedOnDecision: true, common.RunStateActive: true,
			common.RunStateFailed: true, common.RunStateCancelled: true, common.RunStateSuperseded: true,
		},
		common.RunStateBlockedOnApproval: {
			common.RunStateBlockedOnApproval: true, common.RunStateActive: true,
			common.RunStateFailed: true, common.RunStateCancelled: true, common.RunStateSuperseded: true,
		},
		common.RunStateCompleted:  {common.RunStateCompleted: true},
		common.RunStateFailed:     {common.RunStateFailed: true},
		common.RunStateCancelled:  {common.RunStateCancelled: true},
		common.RunStateSuperseded: {common.RunStateSuperseded: true},
	}
	for _, from := range states {
		for _, to := range states {
			want := allowed[from][to]
			if got := runStateCanTransition(from, to); got != want {
				t.Errorf("runStateCanTransition(%q, %q) = %t, want %t", from, to, got, want)
			}
		}
	}
}
