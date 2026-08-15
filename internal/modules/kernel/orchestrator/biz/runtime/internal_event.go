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
	Aggregate               RuntimeAggregate
	CandidateSession        runtimecontract.RuntimeCandidateRequestSession
	PendingAuthorityFailure *runtimecontract.FailureFact
	Admission               *CandidateAdmissionCommitResult
	Evidence                *EvidenceCommitResult
	Receipt                 *runtimecontract.RuntimeOperationReceipt
	Found                   bool
	BudgetConsumed          bool
	DriveLease              *runtimecontract.RuntimeDriveLease
	LeaseChanged            bool
	LeaseClaimed            bool
}

// ApplyInternalEvent is the only mutation entry point exposed by the
// RuntimeController to the Orchestrator Biz. Individual reducer helpers stay
// private implementation detail behind this dispatcher.
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
	result := RuntimeInternalEventResult{}
	switch event.Kind {
	case RuntimeInternalEventActivationObserved:
		if event.Activation.ActivationFacts == nil || event.Activation.ActivationDefinition == nil ||
			event.Activation.ActivationFacts.WorkspaceID != event.Scope.WorkspaceID ||
			event.Activation.ActivationFacts.RunID != event.Scope.RunID {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.initialize(ctx, *event.Activation.ActivationFacts, event.Activation.ActivationDefinition.Clone(), append([]runtimecontract.RuntimeRequirement(nil), event.Activation.ActivationRequirements...))
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconcileAuthority:
		aggregate, err := c.authority.reconcileAuthorityBeforeCandidate(ctx, event.Scope.WorkspaceID, event.Scope.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconciliationRequested:
		if event.Delivery.ReconciliationRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, _, err := c.driveDelivery.ensureReconciliationRequest(ctx, *event.Delivery.ReconciliationRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconciliationBatchObserved:
		if event.Authority.ReconciliationBatch == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, _, err := c.authority.commitReconciliationBatch(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.ReconciliationBatch)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticContractRequired:
		aggregate, err := c.planning.requireSemanticContract(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.SemanticBatchID, event.Authority.SemanticRefreshGeneration)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticCatalogInvalidated:
		if event.Authority.ManifestHash == "" || event.Authority.LogicalRequestID == "" {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.invalidateSemanticCatalogGeneration(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.ManifestHash, event.Authority.LogicalRequestID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCWIBindingRetired:
		if event.Authority.CWIBindingRetirement == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.retireRuntimeCWIBinding(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.CWIBindingRetirement)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventEligibleFocusProjected:
		aggregate, err := c.authority.projectEligibleRuntimeFocus(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.SemanticOrderManifestHash, event.Authority.SemanticCoverageOrder)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticCatalogScopeInvalidated:
		if event.Authority.SemanticCatalogScopeGap == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.invalidateSemanticCatalogForFocusScope(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.SemanticCatalogScopeGap)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanRequestStarted:
		if event.Transaction.SemanticManifest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.startSemanticPlanRequest(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Transaction.SemanticManifest, event.Transaction.StablePrefixHash, event.Transaction.LogicalRequestID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanEvaluated:
		if event.Transaction.SemanticDocument == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.evaluateSemanticPlan(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, *event.Transaction.SemanticDocument)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticFeasibilityRequired:
		aggregate, err := c.planning.requireSemanticFeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticFeasibilityObserved:
		if event.Authority.SemanticFeasibility == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.observeSemanticFeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.SemanticBatchID, *event.Authority.SemanticFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanCorrectionRequired:
		if event.Transaction.SemanticDiagnostic == nil {
			return result, common.ErrInvalidArgument
		}
		if event.Transaction.ExpectedDocument == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.requireSemanticPlanCorrection(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.StablePrefixHash, event.Transaction.LogicalRequestID, *event.Transaction.ExpectedDocument, *event.Transaction.SemanticDiagnostic)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticContractSealed:
		aggregate, err := c.planning.sealSemanticContract(ctx, event.Scope.WorkspaceID, event.Scope.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticContractFailed:
		if event.Authority.SemanticDiagnostic == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.failSemanticContract(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.SemanticFailureCode, *event.Authority.SemanticDiagnostic)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventPrerequisiteObserved:
		if event.Authority.PrerequisiteResult == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.commitPrerequisite(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.PrerequisiteResult)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventPrerequisitesReadOnlyStaleObserved:
		if event.Authority.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.invalidatePrerequisitesForReadOnlyStale(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventFeasibilityObserved:
		if event.Authority.FeasibilityFact == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.recordFeasibilityFact(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.FeasibilityFact)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventFailureObserved:
		if event.Authority.FailureFact == nil {
			return result, common.ErrInvalidArgument
		}
		var aggregate RuntimeAggregate
		var err error
		if event.Authority.CloseActiveDispatch {
			aggregate, err = c.authority.recordDriveFailureFact(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.FailureFact)
		} else {
			aggregate, err = c.authority.recordFailureFact(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.FailureFact)
		}
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventDriveFailureObserved:
		if event.Delivery.FailureFact == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.recordDriveFailureFact(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Delivery.FailureFact)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestEnsured:
		ensured, err := c.planning.ensureCandidateRequestSessionWithExpected(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.DocumentKind, event.Transaction.StablePrefixHash, event.Transaction.ManifestHash, event.Transaction.CorrectionHash, event.Transaction.ExpectedDocument)
		result.Aggregate = ensured.Aggregate
		result.CandidateSession = ensured.Session
		if ensured.AuthorityFailure != nil {
			failure := *ensured.AuthorityFailure
			result.PendingAuthorityFailure = &failure
		}
		return result, err
	case RuntimeInternalEventSemanticPlanBatchCompiled:
		if event.Transaction.SemanticPlanBatch == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.compileSemanticPlanBatch(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Transaction.SemanticPlanBatch)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanBatchAdvanced:
		if event.Transaction.SemanticPlanBatch == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.advanceSemanticPlanBatch(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Transaction.SemanticPlanBatch, event.Transaction.SemanticPlanBatchNextState)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanBatchFeasibilityObserved:
		if event.Transaction.SemanticPlanBatchFeasibilityResult == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.observeSemanticPlanBatchFeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Transaction.SemanticPlanBatchFeasibilityResult)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanBatchDocumentObserved:
		if event.Transaction.SemanticPlanBatchDocument == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.observeSemanticPlanBatchDocument(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Transaction.SemanticPlanBatchDocument)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanBatchDocumentsMerged:
		if event.Transaction.SemanticPlanMergedDocument == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.mergeSemanticPlanBatchDocuments(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Transaction.SemanticPlanMergedDocument)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestSubmitted:
		aggregate, err := c.planning.markCandidateRequestSubmitted(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, event.Transaction.Sequence, event.Transaction.PhysicalCallID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestOutcomeObserved:
		if event.Transaction.RequestOutcome == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.recordCandidateRequestTransportOutcome(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, event.Transaction.PhysicalCallID, *event.Transaction.RequestOutcome)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestRecoveryRequired:
		if event.Transaction.ProviderDocumentOutcome == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.recordCandidateRequestRecoveryRequired(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, event.Transaction.PhysicalCallID, *event.Transaction.ProviderDocumentOutcome)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateDocumentObserved:
		aggregate, err := c.planning.completeCandidateRequestDocument(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, event.Transaction.PhysicalCallID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestStaleObserved:
		if event.Transaction.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.invalidateCandidateRequestSessionForPrerequisiteStale(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, *event.Transaction.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestTerminated:
		if event.Transaction.RequestOutcome == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.terminateCandidateRequestSession(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, *event.Transaction.RequestOutcome)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateAttemptBegun:
		if event.Transaction.Manifest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.beginCandidateAttempt(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Attempt, event.Transaction.StablePrefixHash, *event.Transaction.Manifest, event.Transaction.LogicalRequestID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateDocumentRejected:
		if event.Transaction.Manifest == nil || event.Transaction.CandidateFailure == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.rejectCompletedCandidateDocument(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Attempt, event.Transaction.StablePrefixHash, *event.Transaction.Manifest, event.Transaction.LogicalRequestID, *event.Transaction.CandidateFailure)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateProposalObserved:
		if event.Transaction.Manifest == nil || event.Transaction.Candidate == nil {
			return result, common.ErrInvalidArgument
		}
		admission, err := c.candidateDispatch.admitCandidateProposal(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Attempt, *event.Transaction.Manifest, event.Transaction.LogicalRequestID, *event.Transaction.Candidate)
		result.Aggregate = admission.Aggregate
		result.Admission = &admission
		return result, err
	case RuntimeInternalEventCandidateFeasibilityObserved:
		if event.Transaction.CandidateFeasibility == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.recordCandidateFeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Attempt, *event.Transaction.CandidateFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidatePrerequisiteStaleObserved:
		if event.Transaction.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.invalidateCandidateAttemptForPrerequisiteStale(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Attempt, *event.Transaction.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidatePrerequisiteObserved:
		if event.Transaction.PrerequisiteRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.invalidateCandidateAttemptForFeasibilityPrerequisite(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Attempt, *event.Transaction.PrerequisiteRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventOwnerFeasibilityObserved:
		if event.Authority.OwnerFeasibility == nil || event.Authority.OwnerFeasibility.SealedTargetAuthorizationContradicted() {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.invalidateCandidateAttemptForOwnerInfeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.Attempt, *event.Authority.OwnerFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticBindingContradictionObserved:
		if event.Authority.OwnerFeasibility == nil || !event.Authority.OwnerFeasibility.SealedTargetAuthorizationContradicted() {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.invalidateCandidateAttemptForOwnerInfeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.Attempt, *event.Authority.OwnerFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateSealed:
		if event.Transaction.Manifest == nil || event.Transaction.Candidate == nil {
			return result, common.ErrInvalidArgument
		}
		admission, err := c.candidateDispatch.admitAndSealCandidateAttempt(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Attempt, *event.Transaction.Manifest, event.Transaction.LogicalRequestID, *event.Transaction.Candidate)
		result.Aggregate = admission.Aggregate
		result.Admission = &admission
		return result, err
	case RuntimeInternalEventGovernanceDispatchSealed:
		if event.Transaction.Manifest == nil || event.Transaction.Candidate == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.sealGovernanceDispatch(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Transaction.Manifest, *event.Transaction.Candidate)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRejected:
		if event.Transaction.CandidateFailure == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.failCandidateAttempt(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Attempt, *event.Transaction.CandidateFailure)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventAssessmentObserved:
		aggregate, err := c.authority.commitAssessment(ctx, event.Scope.WorkspaceID, event.Scope.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventGovernanceRequestObserved:
		if event.Authority.GovernanceRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.bindGovernanceRequest(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.GovernanceRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventGovernanceResolutionObserved:
		if event.Transaction.GovernanceResolution == nil {
			return result, common.ErrInvalidArgument
		}
		commit, err := c.authority.commitGovernanceResolution(ctx, *event.Transaction.GovernanceResolution)
		result.Aggregate = commit.Aggregate
		result.Evidence = &commit
		return result, err
	case RuntimeInternalEventOperationReceiptObserved:
		if event.Transaction.OperationReceipt == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, receipt, err := c.candidateDispatch.commitOperationReceipt(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.DispatchID, event.Transaction.Fence, *event.Transaction.OperationReceipt)
		result.Aggregate = aggregate
		result.Receipt = &receipt
		return result, err
	case RuntimeInternalEventValidationFailureObserved:
		// The rejected operation receipt already persisted the capability fact.
		// This event is the explicit owner-local handoff boundary; confirmation
		// below is the first event permitted to turn it into repair authority.
		if event.Authority.FailureFact == nil || event.Authority.FailureFact.ValidationFailureObservation == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.recordValidationFailureObservation(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.FailureFact.ValidationFailureObservation)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventRepairSurfaceConfirmationObserved:
		if event.Authority.RepairSurfaceConfirmation == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.recordRepairSurfaceConfirmation(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.RepairSurfaceConfirmation)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventDispatchExecutionRequested:
		if event.Transaction.Executor == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.executeActiveDispatch(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Executor, event.Transaction.RecoveryOperationID, event.Transaction.RecoveryBudgetAttempt)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventTerminalDeliveryAcknowledged:
		if event.Delivery.DeliveryAcknowledgement == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, changed, err := c.driveDelivery.recordDeliveryAcknowledgement(ctx, *event.Delivery.DeliveryAcknowledgement)
		result.Aggregate = aggregate
		result.BudgetConsumed = changed
		return result, err
	case RuntimeInternalEventRecoveryBudgetConsumed:
		aggregate, consumed, err := c.driveDelivery.consumeRecoveryBudget(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.RecoveryBudgetKind)
		result.Aggregate = aggregate
		result.BudgetConsumed = consumed
		return result, err
	case RuntimeInternalEventDriveLeaseScheduled:
		if event.Delivery.DriveLease == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, lease, changed, err := c.driveDelivery.scheduleDriveLease(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Delivery.DriveLease)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventDriveLeaseClaimed:
		if event.Delivery.DriveLease == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, lease, claimed, err := c.driveDelivery.claimDriveLease(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Delivery.DriveLease, event.Delivery.LeaseNow)
		result.Aggregate, result.LeaseClaimed = aggregate, claimed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventDriveLeaseReleased:
		aggregate, changed, err := c.driveDelivery.releaseDriveLease(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.LeaseGeneration)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		return result, err
	case RuntimeInternalEventDriveLeaseRecovered:
		aggregate, lease, changed, err := c.driveDelivery.recoverDriveLease(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.LeaseNow)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventTerminationRequested:
		aggregate, err := c.driveDelivery.requestTermination(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Delivery.TerminationKind, event.Delivery.RequestedBy, event.Delivery.Reason)
		result.Aggregate = aggregate
		return result, err
	default:
		return result, fmt.Errorf("%w: unsupported runtime internal event %q", common.ErrInvalidArgument, event.Kind)
	}
}
