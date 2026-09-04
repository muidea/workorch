package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
		RuntimeInternalEventProviderRequestRecoveryCoordinated,
		RuntimeInternalEventCandidateDocumentObserved,
		RuntimeInternalEventCandidateRequestStaleObserved,
		RuntimeInternalEventProviderRequestTerminalCoordinated,
		RuntimeInternalEventCandidateAttemptBegun,
		RuntimeInternalEventCandidateDocumentRejected,
		RuntimeInternalEventCandidateProposalObserved,
		RuntimeInternalEventCandidateConformanceObserved,
		RuntimeInternalEventCandidateFeasibilityObserved,
		RuntimeInternalEventCandidatePrerequisiteStaleObserved,
		RuntimeInternalEventCandidatePrerequisiteObserved,
		RuntimeInternalEventCandidateSealed,
		RuntimeInternalEventGovernanceDispatchSealed,
		RuntimeInternalEventCandidateRejected,
		RuntimeInternalEventGovernanceResolutionObserved,
		RuntimeInternalEventOperationReceiptObserved,
		RuntimeInternalEventDispatchExecutionRequested,
		RuntimeInternalEventOperationEvidenceObserved,
		RuntimeInternalEventOperationPrerequisiteStaleObserved,
		RuntimeInternalEventOperationGovernanceRequiredObserved,
		RuntimeInternalEventOperationExecutionOutcomeUnknownObserved,
		RuntimeInternalEventDefinitionClarificationDispatchSealed,
		RuntimeInternalEventDefinitionClarificationRequestObserved:
		return runtimeInternalEventPayloadTransaction, true

	case RuntimeInternalEventReconciliationRequested,
		RuntimeInternalEventDriveFailureObserved,
		RuntimeInternalEventTerminalDeliveryAcknowledged,
		RuntimeInternalEventRecoveryBudgetConsumed,
		RuntimeInternalEventNonTerminalRecoveryScheduled,
		RuntimeInternalEventOperationRecoveryClaimed,
		RuntimeInternalEventOperationRecoveryCallReturned,
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
	if err := event.validateOperationReceiptOwnerSource(); err != nil {
		return err
	}
	if err := event.validateExecutionTransaction(); err != nil {
		return err
	}
	return nil
}

func (event RuntimeInternalEvent) validateExecutionTransaction() error {
	if event.Transaction == nil {
		return nil
	}
	switch event.Kind {
	case RuntimeInternalEventDispatchExecutionRequested:
		if event.Transaction.ExecutionEvidence != nil ||
			event.Transaction.OperationPrerequisiteStaleReceipt != nil || event.Transaction.OperationGovernanceResult != nil || event.Transaction.OperationOutcomeUnknownID != "" {
			return fmt.Errorf("%w: dispatch execution preparation cannot carry a result fact", common.ErrInvalidArgument)
		}
	case RuntimeInternalEventOperationEvidenceObserved:
		if event.Transaction.ExecutionEvidence == nil || event.Transaction.RecoveryClaimID != "" || event.Transaction.RecoveryClaimNow != "" ||
			event.Transaction.OperationPrerequisiteStaleReceipt != nil || event.Transaction.OperationGovernanceResult != nil || event.Transaction.OperationOutcomeUnknownID != "" {
			return fmt.Errorf("%w: operation evidence event requires only an evidence fact", common.ErrInvalidArgument)
		}
	case RuntimeInternalEventOperationPrerequisiteStaleObserved:
		receipt := event.Transaction.OperationPrerequisiteStaleReceipt
		if receipt == nil || receipt.PrerequisiteStale == nil || event.Transaction.DispatchID == "" ||
			event.Transaction.RecoveryClaimID != "" || event.Transaction.RecoveryClaimNow != "" || event.Transaction.ExecutionEvidence != nil ||
			event.Transaction.OperationGovernanceResult != nil || event.Transaction.OperationOutcomeUnknownID != "" {
			return fmt.Errorf("%w: operation prerequisite-stale event requires only dispatch and persisted stale receipt facts", common.ErrInvalidArgument)
		}
	case RuntimeInternalEventOperationGovernanceRequiredObserved:
		result := event.Transaction.OperationGovernanceResult
		if result == nil || result.Governance == nil || result.OperationID == "" ||
			event.Transaction.RecoveryClaimID != "" || event.Transaction.RecoveryClaimNow != "" || event.Transaction.ExecutionEvidence != nil ||
			event.Transaction.OperationPrerequisiteStaleReceipt != nil || event.Transaction.OperationOutcomeUnknownID != "" {
			return fmt.Errorf("%w: operation governance event requires only a persisted governance result fact", common.ErrInvalidArgument)
		}
	case RuntimeInternalEventOperationExecutionOutcomeUnknownObserved:
		if strings.TrimSpace(event.Transaction.OperationOutcomeUnknownID) == "" || event.Transaction.OperationOutcomeUnknownID != strings.TrimSpace(event.Transaction.OperationOutcomeUnknownID) ||
			event.Transaction.RecoveryClaimID != "" || event.Transaction.RecoveryClaimNow != "" || event.Transaction.ExecutionEvidence != nil ||
			event.Transaction.OperationPrerequisiteStaleReceipt != nil || event.Transaction.OperationGovernanceResult != nil {
			return fmt.Errorf("%w: operation outcome-unknown event requires only an exact operation identity", common.ErrInvalidArgument)
		}
	default:
		if event.Transaction.ExecutionEvidence != nil ||
			event.Transaction.OperationPrerequisiteStaleReceipt != nil || event.Transaction.OperationGovernanceResult != nil || event.Transaction.OperationOutcomeUnknownID != "" {
			return fmt.Errorf("%w: runtime internal event %q cannot carry execution orchestration data", common.ErrInvalidArgument, event.Kind)
		}
	}
	return nil
}

// validateOperationReceiptOwnerSource keeps cross-owner producer identity on
// its one owner-private fact. Missing/non-canonical identity is rejected before
// aggregate I/O, and unrelated SM-3 facts cannot smuggle receipt authority.
func (event RuntimeInternalEvent) validateOperationReceiptOwnerSource() error {
	if event.Transaction == nil {
		return nil
	}
	source := event.Transaction.OperationReceiptOwnerSource
	if event.Kind == RuntimeInternalEventOperationReceiptObserved {
		if !exactOperationReceiptOwnerSource(source) {
			return fmt.Errorf("%w: operation receipt event requires exact owner source", common.ErrInvalidArgument)
		}
		return nil
	}
	if source != "" {
		return fmt.Errorf("%w: runtime internal event %q cannot carry operation receipt owner source", common.ErrInvalidArgument, event.Kind)
	}
	return nil
}

// validateRecoveryAuthorization keeps the SM-3 execution transaction and the
// SM-4 recovery-budget handoff separate at the private-envelope boundary.
// Recovery credentials are an exact persisted-counter fence produced by a
// prior RecoveryBudgetConsumed event; they must never be smuggled into another
// transaction fact or partially specified dispatch request.
func (event RuntimeInternalEvent) validateRecoveryAuthorization() error {
	transactionClaimID := ""
	transactionClaimNow := ""
	if event.Transaction != nil {
		transactionClaimID = event.Transaction.RecoveryClaimID
		transactionClaimNow = event.Transaction.RecoveryClaimNow
	}
	if event.Kind == RuntimeInternalEventDispatchExecutionRequested {
		if transactionClaimID != strings.TrimSpace(transactionClaimID) || transactionClaimNow != strings.TrimSpace(transactionClaimNow) ||
			(transactionClaimID != "" && transactionClaimNow == "") {
			return fmt.Errorf("%w: dispatch recovery authorization token is not canonical", common.ErrInvalidArgument)
		}
	} else if transactionClaimID != "" || transactionClaimNow != "" {
		return fmt.Errorf("%w: runtime internal event %q cannot carry a dispatch recovery authorization token", common.ErrInvalidArgument, event.Kind)
	}
	if event.Delivery == nil {
		return nil
	}
	operationID := strings.TrimSpace(event.Delivery.RecoveryOperationID)
	authorizationID := strings.TrimSpace(event.Delivery.RecoveryAuthorizationID)
	claimID := strings.TrimSpace(event.Delivery.RecoveryClaimID)
	terminalRequest := event.Delivery.TerminalDeliveryRecoveryRequest
	nonterminalRequest := event.Delivery.NonTerminalRecoveryRequest
	nonterminalCause := event.Delivery.NonTerminalRecoveryCause
	if event.Kind == RuntimeInternalEventNonTerminalRecoveryScheduled {
		if nonterminalRequest == nil || nonterminalRequest.WorkspaceID != event.Scope.WorkspaceID || nonterminalRequest.RunID != event.Scope.RunID ||
			event.Delivery.ReconciliationRequest != nil || event.Delivery.FailureFact != nil || event.Delivery.DeliveryAcknowledgement != nil ||
			event.Delivery.RecoveryBudgetKind != "" || terminalRequest != nil || operationID != "" || authorizationID != "" || claimID != "" ||
			event.Delivery.DriveLease != nil || event.Delivery.RecoverableWait != nil || strings.TrimSpace(event.Delivery.RecoverableWaitID) != "" ||
			strings.TrimSpace(event.Delivery.RecoverableWaitProbeResultHash) != "" || event.Delivery.LeaseGeneration != 0 ||
			strings.TrimSpace(event.Delivery.LeaseNow) != "" || event.Delivery.TerminationKind != "" ||
			strings.TrimSpace(event.Delivery.RequestedBy) != "" || strings.TrimSpace(event.Delivery.Reason) != "" ||
			validateRuntimeNonTerminalRecoveryCause(nonterminalRequest.Normalized(), nonterminalCause) != nil {
			return fmt.Errorf("%w: nonterminal recovery event requires only an exact typed persistence outcome", common.ErrInvalidArgument)
		}
		return nil
	}
	if nonterminalRequest != nil || nonterminalCause != nil {
		return fmt.Errorf("%w: runtime internal event %q cannot carry nonterminal recovery identity", common.ErrInvalidArgument, event.Kind)
	}
	switch event.Kind {
	case RuntimeInternalEventRecoveryBudgetConsumed:
		if event.Delivery.RecoveryBudgetKind == runtimecontract.RuntimeRecoveryBudgetCASReload {
			return fmt.Errorf("%w: CAS reload budget is owned by the atomic nonterminal recovery event", common.ErrInvalidArgument)
		}
		if event.Delivery.RecoveryBudgetKind == runtimecontract.RuntimeRecoveryBudgetOperationRecovery {
			if operationID == "" || authorizationID != "" || claimID != "" || strings.TrimSpace(event.Delivery.LeaseNow) == "" || terminalRequest != nil {
				return fmt.Errorf("%w: operation recovery budget requires exact operation identity only", common.ErrInvalidArgument)
			}
			return nil
		}
		if operationID != "" || authorizationID != "" || claimID != "" {
			return fmt.Errorf("%w: non-operation recovery budget cannot carry operation authorization", common.ErrInvalidArgument)
		}
		if event.Delivery.RecoveryBudgetKind == runtimecontract.RuntimeRecoveryBudgetTerminalDelivery {
			if terminalRequest == nil || terminalRequest.Validate() != nil {
				return fmt.Errorf("%w: terminal delivery recovery budget requires exact request identity", common.ErrInvalidArgument)
			}
		} else if terminalRequest != nil {
			return fmt.Errorf("%w: non-terminal recovery budget cannot carry terminal request identity", common.ErrInvalidArgument)
		}
	case RuntimeInternalEventOperationRecoveryClaimed:
		if operationID == "" || authorizationID == "" || claimID != "" || event.Delivery.RecoveryBudgetKind != "" || strings.TrimSpace(event.Delivery.LeaseNow) == "" || terminalRequest != nil {
			return fmt.Errorf("%w: operation recovery claim requires operation, authorization and observed-at identity", common.ErrInvalidArgument)
		}
	case RuntimeInternalEventOperationRecoveryCallReturned:
		if operationID == "" || authorizationID == "" || claimID == "" || event.Delivery.RecoveryBudgetKind != "" || event.Delivery.LeaseNow != "" || terminalRequest != nil {
			return fmt.Errorf("%w: operation recovery return requires operation, authorization and claim identity", common.ErrInvalidArgument)
		}
	default:
		if operationID != "" || authorizationID != "" || claimID != "" || terminalRequest != nil {
			return fmt.Errorf("%w: runtime internal event %q cannot carry operation recovery identity", common.ErrInvalidArgument, event.Kind)
		}
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
	descriptor, declared := runtimeInternalTransitionContract(kind)
	if !declared || !descriptor.valid() {
		return eventcontract.Metadata{}, false
	}
	metadata := eventcontract.Metadata{
		Layer:             eventcontract.LayerOrchestratorPrivateFact,
		ReducerOwner:      eventcontract.ReducerOwnerForStateMachine(eventcontract.StateMachineSM2),
		MachineRole:       eventcontract.StateMachineRoleOwnerInternalReducer,
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
		RuntimeInternalEventOperationEvidenceObserved,
		RuntimeInternalEventOperationPrerequisiteStaleObserved,
		RuntimeInternalEventOperationGovernanceRequiredObserved,
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
		RuntimeInternalEventCandidateDocumentObserved,
		RuntimeInternalEventCandidateRequestStaleObserved,
		RuntimeInternalEventProviderRequestTerminalCoordinated,
		RuntimeInternalEventCandidateAttemptBegun,
		RuntimeInternalEventCandidateDocumentRejected,
		RuntimeInternalEventCandidateProposalObserved,
		RuntimeInternalEventCandidateConformanceObserved,
		RuntimeInternalEventCandidateFeasibilityObserved,
		RuntimeInternalEventCandidatePrerequisiteStaleObserved,
		RuntimeInternalEventCandidatePrerequisiteObserved,
		RuntimeInternalEventCandidateSealed,
		RuntimeInternalEventGovernanceDispatchSealed,
		RuntimeInternalEventCandidateRejected,
		RuntimeInternalEventGovernanceResolutionObserved,
		RuntimeInternalEventOperationReceiptObserved,
		RuntimeInternalEventDispatchExecutionRequested,
		RuntimeInternalEventOperationExecutionOutcomeUnknownObserved,
		RuntimeInternalEventDefinitionClarificationDispatchSealed,
		RuntimeInternalEventDefinitionClarificationRequestObserved:
		metadata.StateMachine = eventcontract.StateMachineSM3

	case RuntimeInternalEventReconciliationRequested,
		RuntimeInternalEventProviderRequestRecoveryCoordinated,
		RuntimeInternalEventDriveFailureObserved,
		RuntimeInternalEventTerminalDeliveryAcknowledged,
		RuntimeInternalEventRecoveryBudgetConsumed,
		RuntimeInternalEventNonTerminalRecoveryScheduled,
		RuntimeInternalEventOperationRecoveryClaimed,
		RuntimeInternalEventOperationRecoveryCallReturned,
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
	if metadata.StateMachine != descriptor.Primary {
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
		RuntimeInternalEventProviderRequestRecoveryCoordinated,
		RuntimeInternalEventProviderRequestTerminalCoordinated,
		RuntimeInternalEventCandidateDocumentObserved,
		RuntimeInternalEventCandidateRequestStaleObserved,
		RuntimeInternalEventCandidateDocumentRejected,
		RuntimeInternalEventCandidateProposalObserved,
		RuntimeInternalEventCandidateConformanceObserved,
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
		RuntimeInternalEventOperationEvidenceObserved,
		RuntimeInternalEventOperationPrerequisiteStaleObserved,
		RuntimeInternalEventOperationGovernanceRequiredObserved,
		RuntimeInternalEventOperationExecutionOutcomeUnknownObserved,
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
		RuntimeInternalEventCandidateConformanceObserved,
		RuntimeInternalEventOperationEvidenceObserved,
		RuntimeInternalEventOperationPrerequisiteStaleObserved,
		RuntimeInternalEventOperationGovernanceRequiredObserved,
		RuntimeInternalEventOperationExecutionOutcomeUnknownObserved,
		RuntimeInternalEventValidationFailureObserved,
		RuntimeInternalEventRepairSurfaceConfirmationObserved:
		metadata.IdentityPolicy = eventcontract.IdentityPolicyRuntimeOperation
	case RuntimeInternalEventDispatchExecutionRequested:
		metadata.IdentityPolicy = eventcontract.IdentityPolicyRuntimeDispatch
	}

	return metadata, true
}
