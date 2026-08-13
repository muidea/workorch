package runtime

import "github.com/muidea/workorch/internal/pkg/eventcontract"

// runtimeInternalFactMetadata classifies the Orchestrator owner's private
// transition vocabulary. It is deliberately an exhaustive switch instead of
// a package/global registry: callers cannot look up or dispatch facts through
// it, and no other component can import a shared index to infer authority.
//
// The classification is used only for architecture validation. Runtime state
// still changes exclusively through RuntimeController.ApplyInternalEvent.
func runtimeInternalFactMetadata(kind RuntimeInternalEventKind) (eventcontract.Metadata, bool) {
	metadata := eventcontract.Metadata{
		Layer:             eventcontract.LayerOrchestratorPrivateFact,
		EffectClass:       eventcontract.EffectClassOwnerState,
		SourcePolicy:      eventcontract.SourcePolicyCallerContext,
		ScopePolicy:       eventcontract.ScopePolicyWorkspaceRun,
		IdentityPolicy:    eventcontract.IdentityPolicyRuntimeAuthority,
		TransitionTarget:  eventcontract.TransitionTargetReceiverOwnerPrivate,
		LaneIdentity:      "orchestrator-runtime-private",
		CanonicalFeedback: false,
	}

	switch kind {
	case RuntimeInternalEventActivationObserved,
		RuntimeInternalEventReconcileAuthority,
		RuntimeInternalEventReconciliationBatchObserved,
		RuntimeInternalEventSemanticContractRequired,
		RuntimeInternalEventSemanticCatalogInvalidated,
		RuntimeInternalEventSemanticCatalogScopeInvalidated,
		RuntimeInternalEventCWIBindingRetired,
		RuntimeInternalEventEligibleFocusProjected,
		RuntimeInternalEventSemanticFeasibilityRequired,
		RuntimeInternalEventSemanticFeasibilityObserved,
		RuntimeInternalEventSemanticContractSealed,
		RuntimeInternalEventSemanticContractFailed,
		RuntimeInternalEventPrerequisiteObserved,
		RuntimeInternalEventPrerequisitesReadOnlyStaleObserved,
		RuntimeInternalEventFeasibilityObserved,
		RuntimeInternalEventFailureObserved,
		RuntimeInternalEventOwnerFeasibilityObserved,
		RuntimeInternalEventAssessmentObserved,
		RuntimeInternalEventGovernanceRequestObserved,
		RuntimeInternalEventValidationFailureObserved,
		RuntimeInternalEventRepairSurfaceConfirmationObserved:
		metadata.StateMachine = eventcontract.StateMachineSM2

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
		RuntimeInternalEventCandidateRequestOutcomeObserved,
		RuntimeInternalEventCandidateDocumentObserved,
		RuntimeInternalEventCandidateRequestStaleObserved,
		RuntimeInternalEventCandidateRequestTerminated,
		RuntimeInternalEventCandidateAttemptBegun,
		RuntimeInternalEventCandidateDocumentRejected,
		RuntimeInternalEventCandidateProposalObserved,
		RuntimeInternalEventCandidateFeasibilityObserved,
		RuntimeInternalEventCandidatePrerequisiteStaleObserved,
		RuntimeInternalEventCandidatePrerequisiteObserved,
		RuntimeInternalEventCandidateSealed,
		RuntimeInternalEventGovernanceDispatchSealed,
		RuntimeInternalEventCandidateRejected,
		RuntimeInternalEventGovernanceResolutionObserved,
		RuntimeInternalEventOperationReceiptObserved,
		RuntimeInternalEventDispatchExecutionRequested:
		metadata.StateMachine = eventcontract.StateMachineSM3

	case RuntimeInternalEventReconciliationRequested,
		RuntimeInternalEventCandidateRequestRecoveryRequired,
		RuntimeInternalEventDriveFailureObserved,
		RuntimeInternalEventTerminalDeliveryAcknowledged,
		RuntimeInternalEventRecoveryBudgetConsumed,
		RuntimeInternalEventDriveLeaseScheduled,
		RuntimeInternalEventDriveLeaseClaimed,
		RuntimeInternalEventDriveLeaseReleased,
		RuntimeInternalEventDriveLeaseRecovered,
		RuntimeInternalEventTerminationRequested:
		metadata.StateMachine = eventcontract.StateMachineSM4

	default:
		return eventcontract.Metadata{}, false
	}

	switch kind {
	case RuntimeInternalEventActivationObserved,
		RuntimeInternalEventReconciliationBatchObserved,
		RuntimeInternalEventSemanticPlanEvaluated,
		RuntimeInternalEventSemanticFeasibilityObserved,
		RuntimeInternalEventPrerequisiteObserved,
		RuntimeInternalEventPrerequisitesReadOnlyStaleObserved,
		RuntimeInternalEventFeasibilityObserved,
		RuntimeInternalEventFailureObserved,
		RuntimeInternalEventDriveFailureObserved,
		RuntimeInternalEventCandidateRequestOutcomeObserved,
		RuntimeInternalEventCandidateRequestRecoveryRequired,
		RuntimeInternalEventCandidateDocumentObserved,
		RuntimeInternalEventCandidateRequestStaleObserved,
		RuntimeInternalEventCandidateDocumentRejected,
		RuntimeInternalEventCandidateProposalObserved,
		RuntimeInternalEventCandidateFeasibilityObserved,
		RuntimeInternalEventCandidatePrerequisiteStaleObserved,
		RuntimeInternalEventCandidatePrerequisiteObserved,
		RuntimeInternalEventOwnerFeasibilityObserved,
		RuntimeInternalEventCandidateRejected,
		RuntimeInternalEventAssessmentObserved,
		RuntimeInternalEventGovernanceRequestObserved,
		RuntimeInternalEventGovernanceResolutionObserved,
		RuntimeInternalEventOperationReceiptObserved,
		RuntimeInternalEventValidationFailureObserved,
		RuntimeInternalEventRepairSurfaceConfirmationObserved,
		RuntimeInternalEventTerminalDeliveryAcknowledged:
		metadata.SourcePolicy = eventcontract.SourcePolicyOwnerFact
		metadata.CanonicalFeedback = true
	}

	switch kind {
	case RuntimeInternalEventCandidateSealed,
		RuntimeInternalEventGovernanceDispatchSealed:
		metadata.IdentityPolicy = eventcontract.IdentityPolicyRuntimeDispatch
	case RuntimeInternalEventOperationReceiptObserved,
		RuntimeInternalEventValidationFailureObserved,
		RuntimeInternalEventRepairSurfaceConfirmationObserved:
		metadata.IdentityPolicy = eventcontract.IdentityPolicyRuntimeOperation
	case RuntimeInternalEventDispatchExecutionRequested:
		metadata.IdentityPolicy = eventcontract.IdentityPolicyRuntimeDispatch
		metadata.EffectClass = eventcontract.EffectClassExternalSideEffect
	}

	return metadata, true
}
