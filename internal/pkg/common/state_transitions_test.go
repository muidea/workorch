package common

import "testing"

func TestRunStateCanTransitionMatchesOrchestratorContract(t *testing.T) {
	valid := map[RunState][]RunState{
		RunStatePending:           {RunStateActive, RunStateBlockedOnDecision, RunStateBlockedOnApproval, RunStateCancelled, RunStateFailed},
		RunStateActive:            {RunStateBlockedOnDecision, RunStateBlockedOnApproval, RunStateCompleted, RunStateFailed, RunStateCancelled, RunStateSuperseded},
		RunStateBlockedOnDecision: {RunStateActive, RunStateFailed, RunStateCancelled, RunStateSuperseded},
		RunStateBlockedOnApproval: {RunStateActive, RunStateFailed, RunStateCancelled, RunStateSuperseded},
	}
	for from, targets := range valid {
		for _, to := range targets {
			if !RunStateCanTransition(from, to) {
				t.Fatalf("RunStateCanTransition(%s, %s) = false, want true", from, to)
			}
		}
	}
	for _, terminal := range []RunState{RunStateCompleted, RunStateFailed, RunStateCancelled, RunStateSuperseded} {
		if !RunStateIsTerminal(terminal) {
			t.Fatalf("RunStateIsTerminal(%s) = false, want true", terminal)
		}
		if RunStateCanTransition(terminal, RunStateActive) {
			t.Fatalf("RunStateCanTransition(%s, active) = true, want false", terminal)
		}
	}
}

func TestStepStateCanTransitionMatchesOrchestratorContract(t *testing.T) {
	valid := map[StepState][]StepState{
		StepStatePending:   {StepStateExecuting, StepStateSkipped, StepStateCancelled, StepStateFailed},
		StepStateExecuting: {StepStateSucceeded, StepStateFailed, StepStateCancelled},
	}
	for from, targets := range valid {
		for _, to := range targets {
			if !StepStateCanTransition(from, to) {
				t.Fatalf("StepStateCanTransition(%s, %s) = false, want true", from, to)
			}
		}
	}
	for _, terminal := range []StepState{StepStateSucceeded, StepStateFailed, StepStateSkipped, StepStateCancelled} {
		if !StepStateIsTerminal(terminal) {
			t.Fatalf("StepStateIsTerminal(%s) = false, want true", terminal)
		}
		if StepStateCanTransition(terminal, StepStateExecuting) {
			t.Fatalf("StepStateCanTransition(%s, executing) = true, want false", terminal)
		}
	}
}
