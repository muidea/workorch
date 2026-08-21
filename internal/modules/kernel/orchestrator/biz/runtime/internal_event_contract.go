package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

type runtimeInternalEventPayloadClass string

const (
	runtimeInternalEventPayloadActivation  runtimeInternalEventPayloadClass = "activation"
	runtimeInternalEventPayloadAuthority   runtimeInternalEventPayloadClass = "authority"
	runtimeInternalEventPayloadTransaction runtimeInternalEventPayloadClass = "transaction"
	runtimeInternalEventPayloadDelivery    runtimeInternalEventPayloadClass = "delivery"
)

// runtimeInternalFactPayloadClass binds every owner-private fact kind to one
// concrete payload family. Keep this exhaustive and owner-local: it is an
// envelope invariant, not a cross-owner event registry.
func runtimeInternalFactPayloadClass(kind RuntimeInternalEventKind) (runtimeInternalEventPayloadClass, bool) {
	switch kind {
	case RuntimeInternalEventActivationObserved:
		return runtimeInternalEventPayloadActivation, true

	case RuntimeInternalEventReconcileAuthority,
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
		RuntimeInternalEventSemanticBindingContradictionObserved,
		RuntimeInternalEventAssessmentObserved,
		RuntimeInternalEventGovernanceRequestObserved,
		RuntimeInternalEventValidationFailureObserved,
		RuntimeInternalEventRepairSurfaceConfirmationObserved,
		RuntimeInternalEventDefinitionGapObserved:
		return runtimeInternalEventPayloadAuthority, true

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
		RuntimeInternalEventCandidateRequestRecoveryRequired,
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
		RuntimeInternalEventDispatchExecutionRequested,
		RuntimeInternalEventDefinitionClarificationDispatchSealed,
		RuntimeInternalEventDefinitionClarificationRequestObserved:
		return runtimeInternalEventPayloadTransaction, true

	case RuntimeInternalEventReconciliationRequested,
		RuntimeInternalEventDriveFailureObserved,
		RuntimeInternalEventTerminalDeliveryAcknowledged,
		RuntimeInternalEventRecoveryBudgetConsumed,
		RuntimeInternalEventDriveLeaseScheduled,
		RuntimeInternalEventDriveLeaseClaimed,
		RuntimeInternalEventDriveLeaseReleased,
		RuntimeInternalEventDriveLeaseRecovered,
		RuntimeInternalEventRecoverableWaitOpened,
		RuntimeInternalEventRecoverableWaitProbeDue,
		RuntimeInternalEventRecoverableWaitProbeDeferred,
		RuntimeInternalEventRecoverableWaitExhausted,
		RuntimeInternalEventRecoverableWaitRetired,
		RuntimeInternalEventTerminationRequested:
		return runtimeInternalEventPayloadDelivery, true
	default:
		return "", false
	}
}

func (event RuntimeInternalEvent) validateEnvelope() error {
	if event.Scope.WorkspaceID == "" || event.Scope.RunID == "" {
		return fmt.Errorf("%w: runtime internal event %q has invalid scope", common.ErrInvalidArgument, event.Kind)
	}
	expected, ok := runtimeInternalFactPayloadClass(event.Kind)
	if !ok {
		return fmt.Errorf("%w: runtime internal event %q has no concrete payload classification", common.ErrInvalidArgument, event.Kind)
	}

	payloadCount := 0
	if event.Activation != nil {
		payloadCount++
	}
	if event.Authority != nil {
		payloadCount++
	}
	if event.Transaction != nil {
		payloadCount++
	}
	if event.Delivery != nil {
		payloadCount++
	}
	if payloadCount != 1 {
		return fmt.Errorf("%w: runtime internal event %q must carry exactly one concrete payload, got %d", common.ErrInvalidArgument, event.Kind, payloadCount)
	}

	matched := expected == runtimeInternalEventPayloadActivation && event.Activation != nil ||
		expected == runtimeInternalEventPayloadAuthority && event.Authority != nil ||
		expected == runtimeInternalEventPayloadTransaction && event.Transaction != nil ||
		expected == runtimeInternalEventPayloadDelivery && event.Delivery != nil
	if !matched {
		return fmt.Errorf("%w: runtime internal event %q requires %s payload", common.ErrInvalidArgument, event.Kind, expected)
	}
	if err := event.validateRecoveryAuthorization(); err != nil {
		return err
	}
	return nil
}

// validateRecoveryAuthorization keeps the SM-3 execution transaction and the
// SM-4 recovery-budget handoff separate at the private-envelope boundary.
// Recovery credentials are an exact persisted-counter fence produced by a
// prior RecoveryBudgetConsumed event; they must never be smuggled into another
// transaction fact or partially specified dispatch request.
func (event RuntimeInternalEvent) validateRecoveryAuthorization() error {
	if event.Transaction == nil {
		return nil
	}
	operationID := strings.TrimSpace(event.Transaction.RecoveryOperationID)
	authenticated := operationID != "" || event.Transaction.RecoveryBudgetAttempt != 0
	if event.Kind != RuntimeInternalEventDispatchExecutionRequested {
		if authenticated {
			return fmt.Errorf("%w: runtime internal event %q cannot carry recovery authorization", common.ErrInvalidArgument, event.Kind)
		}
		return nil
	}
	if operationID == "" {
		if event.Transaction.RecoveryBudgetAttempt != 0 {
			return fmt.Errorf("%w: dispatch recovery budget attempt requires recovery operation identity", common.ErrInvalidArgument)
		}
		return nil
	}
	if event.Transaction.RecoveryBudgetAttempt <= 0 {
		return fmt.Errorf("%w: dispatch recovery authorization requires a positive persisted budget attempt", common.ErrInvalidArgument)
	}
	return nil
}

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
		RuntimeInternalEventSemanticBindingContradictionObserved,
		RuntimeInternalEventAssessmentObserved,
		RuntimeInternalEventGovernanceRequestObserved,
		RuntimeInternalEventValidationFailureObserved,
		RuntimeInternalEventRepairSurfaceConfirmationObserved,
		RuntimeInternalEventDefinitionGapObserved:
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
		RuntimeInternalEventDispatchExecutionRequested,
		RuntimeInternalEventDefinitionClarificationDispatchSealed,
		RuntimeInternalEventDefinitionClarificationRequestObserved:
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
		RuntimeInternalEventRecoverableWaitOpened,
		RuntimeInternalEventRecoverableWaitProbeDue,
		RuntimeInternalEventRecoverableWaitProbeDeferred,
		RuntimeInternalEventRecoverableWaitExhausted,
		RuntimeInternalEventRecoverableWaitRetired,
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
		RuntimeInternalEventSemanticBindingContradictionObserved,
		RuntimeInternalEventCandidateRejected,
		RuntimeInternalEventAssessmentObserved,
		RuntimeInternalEventGovernanceRequestObserved,
		RuntimeInternalEventGovernanceResolutionObserved,
		RuntimeInternalEventOperationReceiptObserved,
		RuntimeInternalEventValidationFailureObserved,
		RuntimeInternalEventRepairSurfaceConfirmationObserved,
		RuntimeInternalEventTerminalDeliveryAcknowledged,
		RuntimeInternalEventDefinitionGapObserved,
		RuntimeInternalEventDefinitionClarificationRequestObserved:
		metadata.SourcePolicy = eventcontract.SourcePolicyOwnerFact
		metadata.CanonicalFeedback = true
	}

	switch kind {
	case RuntimeInternalEventCandidateSealed,
		RuntimeInternalEventGovernanceDispatchSealed,
		RuntimeInternalEventDefinitionClarificationDispatchSealed:
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
