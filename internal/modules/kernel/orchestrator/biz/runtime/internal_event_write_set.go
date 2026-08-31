package runtime

import "github.com/muidea/workorch/internal/pkg/eventcontract"

// runtimeInternalStateMachineWriteSet is an owner-private declaration of the
// state-machine domains that one Runtime aggregate CAS is allowed to mutate.
// It is deliberately not exported and does not contain callbacks: external
// components cannot use it as a transition registry or dispatcher.
type runtimeInternalStateMachineWriteSet uint8

const (
	runtimeInternalWriteSM2 runtimeInternalStateMachineWriteSet = 1 << iota
	runtimeInternalWriteSM3
	runtimeInternalWriteSM4
)

type runtimeInternalTransitionDescriptor struct {
	Primary  eventcontract.StateMachine
	WriteSet runtimeInternalStateMachineWriteSet
}

func (d runtimeInternalTransitionDescriptor) valid() bool {
	primary, ok := runtimeInternalWriteBit(d.Primary)
	if !ok || d.Primary == eventcontract.StateMachineSM1 || d.WriteSet == 0 {
		return false
	}
	const allowed = runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4
	return d.WriteSet&^allowed == 0 && d.WriteSet&primary != 0
}

func (d runtimeInternalTransitionDescriptor) writes(machine eventcontract.StateMachine) bool {
	bit, ok := runtimeInternalWriteBit(machine)
	return ok && d.WriteSet&bit != 0
}

func runtimeInternalWriteBit(machine eventcontract.StateMachine) (runtimeInternalStateMachineWriteSet, bool) {
	switch machine {
	case eventcontract.StateMachineSM2:
		return runtimeInternalWriteSM2, true
	case eventcontract.StateMachineSM3:
		return runtimeInternalWriteSM3, true
	case eventcontract.StateMachineSM4:
		return runtimeInternalWriteSM4, true
	default:
		return 0, false
	}
}

// runtimeInternalTransitionContract is exhaustive over the owner-private
// Runtime vocabulary. Primary identifies the machine whose transition caused
// the event; WriteSet is the complete maximum set that the same aggregate CAS
// may coordinate. A grouped case is still an explicit declaration: adding a
// kind anywhere else fails closed until it is placed in exactly one group.
func runtimeInternalTransitionContract(kind RuntimeInternalEventKind) (runtimeInternalTransitionDescriptor, bool) {
	sm2 := runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM2, WriteSet: runtimeInternalWriteSM2}
	sm3 := runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM3, WriteSet: runtimeInternalWriteSM3}
	sm4 := runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM4, WriteSet: runtimeInternalWriteSM4}

	switch kind {
	case RuntimeInternalEventActivationObserved,
		RuntimeInternalEventReconcileAuthority,
		RuntimeInternalEventReconciliationBatchObserved,
		RuntimeInternalEventSemanticCatalogScopeInvalidated,
		RuntimeInternalEventCWIBindingRetired,
		RuntimeInternalEventEligibleFocusProjected,
		RuntimeInternalEventFeasibilityObserved,
		RuntimeInternalEventAssessmentObserved,
		RuntimeInternalEventGovernanceRequestObserved,
		RuntimeInternalEventValidationFailureObserved,
		RuntimeInternalEventRepairSurfaceConfirmationObserved,
		RuntimeInternalEventDefinitionGapObserved:
		return sm2, true

	case RuntimeInternalEventSemanticContractRequired:
		// A catalog refresh retires the SM-3 semantic transaction, rewrites
		// SM-2 authority and consumes the bounded SM-4 refresh ledger.
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM2, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4}, true

	case RuntimeInternalEventSemanticCatalogInvalidated,
		RuntimeInternalEventSemanticFeasibilityRequired,
		RuntimeInternalEventSemanticFeasibilityObserved,
		RuntimeInternalEventSemanticContractSealed,
		RuntimeInternalEventSemanticContractFailed,
		RuntimeInternalEventFailureObserved,
		RuntimeInternalEventOwnerFeasibilityObserved,
		RuntimeInternalEventSemanticBindingContradictionObserved:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM2, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3}, true

	case RuntimeInternalEventPrerequisiteObserved:
		// A successful wait probe can close SM-4 wait/budget lineage while
		// reacquisition retires SM-3 semantic work and advances SM-2 authority.
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM2, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4}, true

	case RuntimeInternalEventPrerequisitesReadOnlyStaleObserved:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM2, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM4}, true

	case RuntimeInternalEventSemanticPlanRequestStarted,
		RuntimeInternalEventSemanticPlanBatchCompiled,
		RuntimeInternalEventSemanticPlanBatchAdvanced,
		RuntimeInternalEventSemanticPlanBatchFeasibilityObserved,
		RuntimeInternalEventSemanticPlanBatchDocumentObserved,
		RuntimeInternalEventSemanticPlanBatchDocumentsMerged,
		RuntimeInternalEventSemanticPlanEvaluated,
		RuntimeInternalEventSemanticPlanCorrectionRequired,
		RuntimeInternalEventCandidateRequestEnsured,
		RuntimeInternalEventCandidateRequestSubmitted,
		RuntimeInternalEventCandidateDocumentObserved,
		RuntimeInternalEventCandidateAttemptBegun,
		RuntimeInternalEventCandidateFeasibilityObserved,
		RuntimeInternalEventGovernanceDispatchSealed,
		RuntimeInternalEventDispatchExecutionRequested,
		RuntimeInternalEventDefinitionClarificationDispatchSealed,
		RuntimeInternalEventDefinitionClarificationRequestObserved:
		return sm3, true

	case RuntimeInternalEventOperationReceiptObserved:
		// The owner receipt advances the SM-3 execution ledger and, when the
		// physical call consumed an in-flight replay claim, settles that exact
		// SM-4 authorization in the same CAS. Leaving the claim in-flight after a
		// durable receipt would retain a false recovery blocker after restart.
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM3, WriteSet: runtimeInternalWriteSM3 | runtimeInternalWriteSM4}, true

	case RuntimeInternalEventOperationEvidenceObserved:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM2, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3}, true

	case RuntimeInternalEventOperationPrerequisiteStaleObserved:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM2, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4}, true

	case RuntimeInternalEventOperationGovernanceRequiredObserved:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM2, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3}, true

	case RuntimeInternalEventOperationExecutionOutcomeUnknownObserved:
		return sm3, true

	case RuntimeInternalEventCandidateDocumentRejected,
		RuntimeInternalEventCandidateProposalObserved,
		RuntimeInternalEventCandidatePrerequisiteObserved,
		RuntimeInternalEventCandidateSealed,
		RuntimeInternalEventCandidateRejected:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM3, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3}, true

	case RuntimeInternalEventCandidateRequestStaleObserved,
		RuntimeInternalEventCandidatePrerequisiteStaleObserved:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM3, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4}, true

	case RuntimeInternalEventProviderRequestTerminalCoordinated:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM3, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3}, true

	case RuntimeInternalEventGovernanceResolutionObserved:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM3, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4}, true

	case RuntimeInternalEventReconciliationRequested,
		RuntimeInternalEventTerminalDeliveryAcknowledged,
		RuntimeInternalEventDriveLeaseScheduled,
		RuntimeInternalEventDriveLeaseClaimed,
		RuntimeInternalEventDriveLeaseReleased,
		RuntimeInternalEventDriveLeaseRecovered,
		RuntimeInternalEventRecoverableWaitOpened,
		RuntimeInternalEventRecoverableWaitRetired,
		RuntimeInternalEventTerminationRequested:
		return sm4, true

	case RuntimeInternalEventRecoverableWaitProbeDue,
		RuntimeInternalEventRecoverableWaitProbeDeferred,
		RuntimeInternalEventRecoverableWaitExhausted:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM4, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM4}, true

	case RuntimeInternalEventProviderRequestRecoveryCoordinated,
		RuntimeInternalEventDriveFailureObserved:
		return runtimeInternalTransitionDescriptor{Primary: eventcontract.StateMachineSM4, WriteSet: runtimeInternalWriteSM2 | runtimeInternalWriteSM3 | runtimeInternalWriteSM4}, true

	case RuntimeInternalEventRecoveryBudgetConsumed,
		RuntimeInternalEventOperationRecoveryClaimed,
		RuntimeInternalEventOperationRecoveryCallReturned:
		// The authorization is SM-4-owned recovery state. Its exact SM-3
		// execution identity is a read fence, not permission to mutate SM-3.
		return sm4, true

	default:
		return runtimeInternalTransitionDescriptor{}, false
	}
}
