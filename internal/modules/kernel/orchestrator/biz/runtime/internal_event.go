package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeInternalEvent is the owner-local input vocabulary for the
// Orchestrator aggregate. It intentionally never crosses EventHub: capability
// owners publish their own typed results, and Biz maps those results to one of
// these events only after it has checked scope, causation and sealed identity.
//
// The event carries facts, not a requested Runtime action. RuntimeController
// remains the sole place that reduces a fact into the next SM-2/SM-3/SM-4
// state, and RuntimeTransitionCoordinator remains the sole CAS boundary.
type RuntimeInternalEvent struct {
	Kind  RuntimeInternalEventKind
	Scope RuntimeInternalEventScope

	Activation  *RuntimeInternalActivationPayload
	Authority   *RuntimeInternalAuthorityPayload
	Transaction *RuntimeInternalTransactionPayload
	Delivery    *RuntimeInternalDeliveryPayload
}

// RuntimeInternalEventScope is the mandatory owner-local aggregate scope. It
// is deliberately separate from every concrete payload so payload fields can
// never become an alternate workspace/run identity.
type RuntimeInternalEventScope struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type RuntimeInternalEventKind string

// RuntimeInternalEventResult is deliberately a bounded owner-local result.
// A caller receives the post-transition aggregate (and, where applicable, an
// immutable receipt/admission result); it never gains repository/CAS access.
type RuntimeInternalEventResult struct {
	Aggregate                 RuntimeAggregate
	CandidateSession          runtimecontract.RuntimeCandidateRequestSession
	PendingAuthorityFailure   *runtimecontract.FailureFact
	Admission                 *CandidateAdmissionCommitResult
	Evidence                  *EvidenceCommitResult
	Receipt                   *runtimecontract.RuntimeOperationReceipt
	OperationCall             *RuntimeOperationExecutionCallHandoff
	DispatchExecution         *RuntimeDispatchExecutionHandoff
	Found                     bool
	BudgetConsumed            bool
	NonTerminalRecoveryRecord *runtimecontract.RuntimeNonTerminalRecoveryRecord
	RecoveryClaimChanged      bool
	RecoveryClaimID           string
	DriveLease                *runtimecontract.RuntimeDriveLease
	LeaseChanged              bool
	LeaseClaimed              bool
}

// ApplyInternalEvent is the only mutation entry point exposed by the
// RuntimeController to the Orchestrator Biz. It validates the owner-private
// envelope and remains the exhaustive kind gate; concrete reducers stay behind
// four private payload-family routes that share the same mutation kernel.
func (c *RuntimeController) ApplyInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	if c == nil || c.runtimeMutationKernel == nil || c.repository == nil || c.authority == nil || c.planning == nil || c.candidateDispatch == nil || c.driveDelivery == nil {
		return RuntimeInternalEventResult{}, common.ErrInvalidArgument
	}
	// Every mutation must enter through the owner-private SM-2/SM-3/SM-4
	// vocabulary. Keep this guard at the dispatcher boundary, before any
	// reducer or repository work, so a newly added event cannot silently become
	// a cross-machine transition merely because it was added to a switch below.
	if metadata, ok := runtimeInternalFactMetadata(event.Kind); !ok || metadata.TransitionTarget != eventcontract.TransitionTargetReceiverOwnerPrivate {
		return RuntimeInternalEventResult{}, fmt.Errorf("%w: runtime internal event %q is not classified as an owner-private transition", common.ErrInvalidArgument, event.Kind)
	}
	if err := event.validateEnvelope(); err != nil {
		return RuntimeInternalEventResult{}, err
	}

	// Keep every concrete kind at this one exported gate. Grouped cases lower
	// dispatcher control-flow complexity without creating a registry, callback
	// transaction, or alternate mutation authority.
	switch event.Kind {
	case RuntimeInternalEventActivationObserved:
		return c.applyActivationInternalEvent(ctx, event)

	case RuntimeInternalEventImplementationWorkScopeRequired,
		RuntimeInternalEventImplementationWorkScopeObserved,
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
		return c.applyAuthorityInternalEvent(ctx, event)

	case RuntimeInternalEventSemanticPlanRequestStarted,
		RuntimeInternalEventSemanticPlanEvaluated,
		RuntimeInternalEventSemanticPlanCorrectionRequired,
		RuntimeInternalEventCandidateRequestEnsured,
		RuntimeInternalEventSemanticPlanBatchCompiled,
		RuntimeInternalEventSemanticPlanBatchAdvanced,
		RuntimeInternalEventSemanticPlanBatchFeasibilityObserved,
		RuntimeInternalEventSemanticPlanBatchDocumentObserved,
		RuntimeInternalEventSemanticPlanBatchDocumentsMerged,
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
		return c.applyTransactionInternalEvent(ctx, event)

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
		return c.applyDeliveryInternalEvent(ctx, event)
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}

func unsupportedRuntimeInternalEvent(kind RuntimeInternalEventKind) (RuntimeInternalEventResult, error) {
	return RuntimeInternalEventResult{}, fmt.Errorf("%w: unsupported runtime internal event %q", common.ErrInvalidArgument, kind)
}
