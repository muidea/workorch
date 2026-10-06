package runtime

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestRuntimeInternalTransitionWriteSetIsExhaustiveAndMatchesPrimaryMetadata(t *testing.T) {
	kinds, _, _ := runtimeInternalEventSourceFacts(t)
	for name, kind := range kinds {
		descriptor, ok := runtimeInternalTransitionContract(kind)
		if !ok {
			t.Fatalf("runtime internal fact %s (%q) has no coordinated write-set declaration", name, kind)
		}
		if !descriptor.valid() {
			t.Fatalf("runtime internal fact %s has invalid transition descriptor: %+v", name, descriptor)
		}
		if descriptor.Primary == eventcontract.StateMachineSM1 || descriptor.writes(eventcontract.StateMachineSM1) {
			t.Fatalf("Orchestrator internal fact %s claims forbidden SM-1 authority: %+v", name, descriptor)
		}
		if !descriptor.writes(descriptor.Primary) {
			t.Fatalf("runtime internal fact %s primary %s is absent from write-set", name, descriptor.Primary)
		}
		metadata, classified := runtimeInternalFactMetadata(kind)
		if !classified {
			t.Fatalf("runtime internal fact %s has no owner-private metadata", name)
		}
		if descriptor.Primary != metadata.StateMachine {
			t.Fatalf("runtime internal fact %s primary = %s, metadata = %s", name, descriptor.Primary, metadata.StateMachine)
		}
	}

	if _, ok := runtimeInternalTransitionContract(RuntimeInternalEventKind("future_undeclared_fact")); ok {
		t.Fatal("an undeclared Runtime internal fact acquired a default write-set")
	}
}

func TestRuntimeInternalCoordinatedTransactionsDeclareExactMachineFamilies(t *testing.T) {
	tests := []struct {
		name    string
		kind    RuntimeInternalEventKind
		primary eventcontract.StateMachine
		writes  runtimeInternalStateMachineWriteSet
	}{
		{name: "provider recovery", kind: RuntimeInternalEventProviderRequestRecoveryCoordinated, primary: eventcontract.StateMachineSM4, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4},
		{name: "provider terminal", kind: RuntimeInternalEventProviderRequestTerminalCoordinated, primary: eventcontract.StateMachineSM3, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3},
		{name: "drive failure", kind: RuntimeInternalEventDriveFailureObserved, primary: eventcontract.StateMachineSM4, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4},
		{name: "governance resolution", kind: RuntimeInternalEventGovernanceResolutionObserved, primary: eventcontract.StateMachineSM3, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4},
		{name: "implementation purpose order projection", kind: RuntimeInternalEventEligibleFocusProjected, primary: eventcontract.StateMachineSM2, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3},
		{name: "owner infeasibility", kind: RuntimeInternalEventOwnerFeasibilityObserved, primary: eventcontract.StateMachineSM2, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3},
		{name: "binding contradiction", kind: RuntimeInternalEventSemanticBindingContradictionObserved, primary: eventcontract.StateMachineSM2, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3},
		{name: "candidate rejected", kind: RuntimeInternalEventCandidateRejected, primary: eventcontract.StateMachineSM3, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3},
		{name: "candidate stale", kind: RuntimeInternalEventCandidatePrerequisiteStaleObserved, primary: eventcontract.StateMachineSM3, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4},
		{name: "candidate feasibility", kind: RuntimeInternalEventCandidateFeasibilityObserved, primary: eventcontract.StateMachineSM3, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4},
		{name: "operation receipt settles work and retires replay authorization", kind: RuntimeInternalEventOperationReceiptObserved, primary: eventcontract.StateMachineSM3, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4},
		{name: "implementation physical unknown closes work and retires replay", kind: RuntimeInternalEventOperationExecutionOutcomeUnknownObserved, primary: eventcontract.StateMachineSM3, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4},
		{name: "operation recovery budget", kind: RuntimeInternalEventRecoveryBudgetConsumed, primary: eventcontract.StateMachineSM4, writes: runtimeInternalWriteSM4},
		{name: "recoverable wait exhaustion", kind: RuntimeInternalEventRecoverableWaitExhausted, primary: eventcontract.StateMachineSM4, writes: runtimeInternalWriteSM2 | runtimeInternalWriteSM4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor, ok := runtimeInternalTransitionContract(test.kind)
			if !ok {
				t.Fatalf("missing coordinated write-set for %q", test.kind)
			}
			if descriptor.Primary != test.primary || descriptor.WriteSet != test.writes {
				t.Fatalf("descriptor = %+v, want primary=%s write-set=%03b", descriptor, test.primary, test.writes)
			}
		})
	}
}

func TestRuntimeInternalTransitionDescriptorRejectsForbiddenShapes(t *testing.T) {
	tests := []runtimeInternalTransitionDescriptor{
		{},
		{Primary: eventcontract.StateMachineSM1, WriteSet: runtimeInternalWriteSM2},
		{Primary: eventcontract.StateMachineSM2, WriteSet: runtimeInternalWriteSM3},
		{Primary: eventcontract.StateMachineSM4, WriteSet: runtimeInternalWriteSM4 | 1<<7},
	}
	for _, descriptor := range tests {
		if descriptor.valid() {
			t.Fatalf("forbidden transition descriptor validated: %+v", descriptor)
		}
	}
}

func TestDispatchPreparationInternalFactDoesNotClaimCapabilitySideEffect(t *testing.T) {
	metadata, ok := runtimeInternalFactMetadata(RuntimeInternalEventDispatchExecutionRequested)
	if !ok {
		t.Fatal("dispatch preparation has no owner-private metadata")
	}
	if metadata.EffectClass != eventcontract.EffectClassOwnerState {
		t.Fatalf("dispatch preparation effect class = %q, want owner-local state only", metadata.EffectClass)
	}
}
