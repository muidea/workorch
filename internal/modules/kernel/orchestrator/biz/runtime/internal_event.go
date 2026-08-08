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
	Kind        RuntimeInternalEventKind
	WorkspaceID common.WorkspaceID
	RunID       common.RunID

	// ActivationFacts and ActivationDefinition are the validated Run-owner
	// bootstrap fact for the one-time Runtime aggregate creation.  They are
	// deliberately carried through the same private dispatcher as every later
	// mutation; a Run notification never calls a controller reducer directly.
	ActivationFacts        *runtimecontract.RuntimeFacts
	ActivationDefinition   *runtimecontract.RuntimeDefinitionContext
	ActivationRequirements []runtimecontract.RuntimeRequirement

	ReconciliationRequest              *runtimecontract.RuntimeCWIReconciliationRequest
	ReconciliationBatch                *runtimecontract.RuntimeCWIReconciliationBatch
	SemanticManifest                   *runtimecontract.RuntimeSemanticPlanManifest
	SemanticDocument                   *runtimecontract.RuntimeSemanticPlanDocument
	SemanticPlanBatch                  *runtimecontract.RuntimeSemanticPlanBatch
	SemanticPlanBatchNextState         runtimecontract.RuntimeSemanticPlanBatchState
	SemanticPlanBatchFeasibilityResult *runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult
	SemanticPlanBatchDocument          *runtimecontract.RuntimeSemanticPlanDocument
	SemanticPlanMergedDocument         *runtimecontract.RuntimeSemanticPlanDocument
	SemanticPlanMergedFeasibilities    []runtimecontract.RuntimeSemanticProofFeasibility
	SemanticFeasibility                *runtimecontract.RuntimeSemanticProofFeasibility
	SemanticDiagnostic                 *runtimecontract.RuntimeOriginalDiagnostic
	SemanticCorrectionDirective        *runtimecontract.RuntimeSemanticCorrectionDirective
	SemanticBatchID                    string
	SemanticRefreshGeneration          int

	PrerequisiteResult  *PrerequisiteResult
	Stale               *runtimecontract.RuntimePrerequisiteStale
	FeasibilityFact     *runtimecontract.RuntimeFeasibilityFact
	FailureFact         *runtimecontract.FailureFact
	CloseActiveDispatch bool

	StablePrefixHash        string
	DocumentKind            runtimecontract.RuntimeProviderDocumentKind
	ManifestHash            string
	CorrectionHash          string
	LogicalRequestID        string
	Sequence                int
	PhysicalCallID          string
	RequestOutcome          *runtimecontract.RuntimeCandidateRequestOutcome
	ProviderDocumentOutcome *runtimecontract.RuntimeProviderDocumentOutcome
	ExpectedDocument        *runtimecontract.RuntimeExpectedDocumentContract

	Attempt                   int
	Manifest                  *runtimecontract.RuntimeCandidateContractManifest
	Candidate                 *runtimecontract.RuntimeCandidate
	CandidateFailure          *runtimecontract.RuntimeCandidateFailure
	CandidateFeasibility      *runtimecontract.RuntimeCandidateFeasibilityRecord
	PrerequisiteRequest       *runtimecontract.RuntimeOperationPrerequisiteRequest
	OwnerFeasibility          *runtimecontract.RuntimeOwnerFeasibilityObservation
	RepairSurfaceConfirmation *runtimecontract.RuntimeRepairSurfaceConfirmation

	OperationReceipt *runtimecontract.RuntimeOperationReceipt
	DispatchID       string
	Fence            uint64
	Executor         RuntimeOperationExecutor

	GovernanceRequest       *runtimecontract.RuntimeGovernanceRequestRecord
	GovernanceResolution    *runtimecontract.RuntimeGovernanceResolution
	DeliveryAcknowledgement *runtimecontract.RuntimeDeliveryAcknowledgement
	RecoveryBudgetKind      runtimecontract.RuntimeRecoveryBudgetKind
	DriveLease              *runtimecontract.RuntimeDriveLease
	LeaseGeneration         uint64
	LeaseNow                string
	TerminationKind         runtimecontract.RuntimeTerminationKind
	RequestedBy             string
	Reason                  string
}

type RuntimeInternalEventKind string

// RuntimeInternalEventResult is deliberately a bounded owner-local result.
// A caller receives the post-transition aggregate (and, where applicable, an
// immutable receipt/admission result); it never gains repository/CAS access.
type RuntimeInternalEventResult struct {
	Aggregate        RuntimeAggregate
	CandidateSession runtimecontract.RuntimeCandidateRequestSession
	Admission        *CandidateAdmissionCommitResult
	Evidence         *EvidenceCommitResult
	Receipt          *runtimecontract.RuntimeOperationReceipt
	Found            bool
	BudgetConsumed   bool
	DriveLease       *runtimecontract.RuntimeDriveLease
	LeaseChanged     bool
	LeaseClaimed     bool
}

// ApplyInternalEvent is the only mutation entry point exposed by the
// RuntimeController to the Orchestrator Biz. Individual reducer helpers stay
// private implementation detail behind this dispatcher.
func (c *RuntimeController) ApplyInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	if c == nil || c.repository == nil || event.WorkspaceID == "" || event.RunID == "" {
		return RuntimeInternalEventResult{}, common.ErrInvalidArgument
	}
	// Every mutation must enter through the owner-private SM-2/SM-3/SM-4
	// vocabulary. Keep this guard at the dispatcher boundary, before any
	// reducer or repository work, so a newly added event cannot silently become
	// a cross-machine transition merely because it was added to a switch below.
	if metadata, ok := runtimeInternalFactMetadata(event.Kind); !ok || metadata.TransitionTarget != eventcontract.TransitionTargetReceiverOwnerPrivate {
		return RuntimeInternalEventResult{}, fmt.Errorf("%w: runtime internal event %q is not classified as an owner-private transition", common.ErrInvalidArgument, event.Kind)
	}
	result := RuntimeInternalEventResult{}
	switch event.Kind {
	case RuntimeInternalEventActivationObserved:
		if event.ActivationFacts == nil || event.ActivationDefinition == nil ||
			event.ActivationFacts.WorkspaceID != event.WorkspaceID || event.ActivationFacts.RunID != event.RunID {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.initialize(ctx, *event.ActivationFacts, event.ActivationDefinition.Clone(), append([]runtimecontract.RuntimeRequirement(nil), event.ActivationRequirements...))
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconcileAuthority:
		aggregate, err := c.reconcileAuthorityBeforeCandidate(ctx, event.WorkspaceID, event.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconciliationRequested:
		if event.ReconciliationRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, _, err := c.ensureReconciliationRequest(ctx, *event.ReconciliationRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconciliationBatchObserved:
		if event.ReconciliationBatch == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, _, err := c.commitReconciliationBatch(ctx, event.WorkspaceID, event.RunID, *event.ReconciliationBatch)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticContractRequired:
		aggregate, err := c.requireSemanticContract(ctx, event.WorkspaceID, event.RunID, event.SemanticBatchID, event.SemanticRefreshGeneration)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanRequestStarted:
		if event.SemanticManifest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.startSemanticPlanRequest(ctx, event.WorkspaceID, event.RunID, *event.SemanticManifest, event.StablePrefixHash, event.LogicalRequestID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanEvaluated:
		if event.SemanticDocument == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.evaluateSemanticPlan(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, *event.SemanticDocument)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticFeasibilityRequired:
		aggregate, err := c.requireSemanticFeasibility(ctx, event.WorkspaceID, event.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticFeasibilityObserved:
		if event.SemanticFeasibility == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.observeSemanticFeasibility(ctx, event.WorkspaceID, event.RunID, event.SemanticBatchID, *event.SemanticFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanCorrectionRequired:
		if event.SemanticDiagnostic == nil || event.SemanticCorrectionDirective == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.requireSemanticPlanCorrection(ctx, event.WorkspaceID, event.RunID, event.StablePrefixHash, event.LogicalRequestID, *event.SemanticCorrectionDirective, *event.SemanticDiagnostic)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticContractSealed:
		aggregate, err := c.sealSemanticContract(ctx, event.WorkspaceID, event.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticContractFailed:
		if event.SemanticDiagnostic == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.failSemanticContract(ctx, event.WorkspaceID, event.RunID, *event.SemanticDiagnostic)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventPrerequisiteObserved:
		if event.PrerequisiteResult == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.commitPrerequisite(ctx, event.WorkspaceID, event.RunID, *event.PrerequisiteResult)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventPrerequisitesReadOnlyStaleObserved:
		if event.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.invalidatePrerequisitesForReadOnlyStale(ctx, event.WorkspaceID, event.RunID, *event.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventFeasibilityObserved:
		if event.FeasibilityFact == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.recordFeasibilityFact(ctx, event.WorkspaceID, event.RunID, *event.FeasibilityFact)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventFailureObserved, RuntimeInternalEventDriveFailureObserved:
		if event.FailureFact == nil {
			return result, common.ErrInvalidArgument
		}
		var aggregate RuntimeAggregate
		var err error
		if event.Kind == RuntimeInternalEventDriveFailureObserved || event.CloseActiveDispatch {
			aggregate, err = c.recordDriveFailureFact(ctx, event.WorkspaceID, event.RunID, *event.FailureFact)
		} else {
			aggregate, err = c.recordFailureFact(ctx, event.WorkspaceID, event.RunID, *event.FailureFact)
		}
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestEnsured:
		aggregate, session, err := c.ensureCandidateRequestSessionWithExpected(ctx, event.WorkspaceID, event.RunID, event.DocumentKind, event.StablePrefixHash, event.ManifestHash, event.CorrectionHash, event.ExpectedDocument)
		result.Aggregate = aggregate
		result.CandidateSession = session
		return result, err
	case RuntimeInternalEventSemanticPlanBatchCompiled:
		if event.SemanticPlanBatch == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.compileSemanticPlanBatch(ctx, event.WorkspaceID, event.RunID, *event.SemanticPlanBatch)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanBatchAdvanced:
		if event.SemanticPlanBatch == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.advanceSemanticPlanBatch(ctx, event.WorkspaceID, event.RunID, *event.SemanticPlanBatch, event.SemanticPlanBatchNextState)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanBatchFeasibilityObserved:
		if event.SemanticPlanBatchFeasibilityResult == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.observeSemanticPlanBatchFeasibility(ctx, event.WorkspaceID, event.RunID, *event.SemanticPlanBatchFeasibilityResult)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanBatchDocumentObserved:
		if event.SemanticPlanBatchDocument == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.observeSemanticPlanBatchDocument(ctx, event.WorkspaceID, event.RunID, *event.SemanticPlanBatchDocument)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanBatchDocumentsMerged:
		if event.SemanticPlanMergedDocument == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.mergeSemanticPlanBatchDocuments(ctx, event.WorkspaceID, event.RunID, *event.SemanticPlanMergedDocument)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticPlanBatchFeasibilitiesMerged:
		aggregate, err := c.mergeSemanticPlanBatchFeasibilities(ctx, event.WorkspaceID, event.RunID, event.SemanticPlanMergedFeasibilities)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestSubmitted:
		aggregate, err := c.markCandidateRequestSubmitted(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, event.Sequence, event.PhysicalCallID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestOutcomeObserved:
		if event.RequestOutcome == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.recordCandidateRequestTransportOutcome(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, event.PhysicalCallID, *event.RequestOutcome)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestRecoveryRequired:
		if event.ProviderDocumentOutcome == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.recordCandidateRequestRecoveryRequired(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, event.PhysicalCallID, *event.ProviderDocumentOutcome)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateDocumentObserved:
		aggregate, err := c.completeCandidateRequestDocument(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, event.PhysicalCallID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestStaleObserved:
		if event.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.invalidateCandidateRequestSessionForPrerequisiteStale(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, *event.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestTerminated:
		if event.RequestOutcome == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.terminateCandidateRequestSession(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, *event.RequestOutcome)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateAttemptBegun:
		if event.Manifest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.beginCandidateAttempt(ctx, event.WorkspaceID, event.RunID, event.Attempt, event.StablePrefixHash, *event.Manifest, event.LogicalRequestID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateDocumentRejected:
		if event.Manifest == nil || event.CandidateFailure == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.rejectCompletedCandidateDocument(ctx, event.WorkspaceID, event.RunID, event.Attempt, event.StablePrefixHash, *event.Manifest, event.LogicalRequestID, *event.CandidateFailure)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateProposalObserved:
		if event.Manifest == nil || event.Candidate == nil {
			return result, common.ErrInvalidArgument
		}
		admission, err := c.admitCandidateProposal(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.Manifest, event.LogicalRequestID, *event.Candidate)
		result.Aggregate = admission.Aggregate
		result.Admission = &admission
		return result, err
	case RuntimeInternalEventCandidateFeasibilityObserved:
		if event.CandidateFeasibility == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.recordCandidateFeasibility(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.CandidateFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidatePrerequisiteStaleObserved:
		if event.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.invalidateCandidateAttemptForPrerequisiteStale(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidatePrerequisiteObserved:
		if event.PrerequisiteRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.invalidateCandidateAttemptForFeasibilityPrerequisite(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.PrerequisiteRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventOwnerFeasibilityObserved:
		if event.OwnerFeasibility == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.invalidateCandidateAttemptForOwnerInfeasibility(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.OwnerFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateSealed:
		if event.Manifest == nil || event.Candidate == nil {
			return result, common.ErrInvalidArgument
		}
		admission, err := c.admitAndSealCandidateAttempt(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.Manifest, event.LogicalRequestID, *event.Candidate)
		result.Aggregate = admission.Aggregate
		result.Admission = &admission
		return result, err
	case RuntimeInternalEventGovernanceDispatchSealed:
		if event.Manifest == nil || event.Candidate == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.sealGovernanceDispatch(ctx, event.WorkspaceID, event.RunID, *event.Manifest, *event.Candidate)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRejected:
		if event.CandidateFailure == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.failCandidateAttempt(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.CandidateFailure)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventAssessmentObserved:
		aggregate, err := c.commitAssessment(ctx, event.WorkspaceID, event.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventGovernanceRequestObserved:
		if event.GovernanceRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.bindGovernanceRequest(ctx, event.WorkspaceID, event.RunID, *event.GovernanceRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventGovernanceResolutionObserved:
		if event.GovernanceResolution == nil {
			return result, common.ErrInvalidArgument
		}
		commit, err := c.commitGovernanceResolution(ctx, *event.GovernanceResolution)
		result.Aggregate = commit.Aggregate
		result.Evidence = &commit
		return result, err
	case RuntimeInternalEventOperationReceiptObserved:
		if event.OperationReceipt == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, receipt, err := c.commitOperationReceipt(ctx, event.WorkspaceID, event.RunID, event.DispatchID, event.Fence, *event.OperationReceipt)
		result.Aggregate = aggregate
		result.Receipt = &receipt
		return result, err
	case RuntimeInternalEventValidationFailureObserved:
		// The rejected operation receipt already persisted the capability fact.
		// This event is the explicit owner-local handoff boundary; confirmation
		// below is the first event permitted to turn it into repair authority.
		if event.FailureFact == nil || event.FailureFact.ValidationFailureObservation == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.recordValidationFailureObservation(ctx, event.WorkspaceID, event.RunID, *event.FailureFact.ValidationFailureObservation)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventRepairSurfaceConfirmationObserved:
		if event.RepairSurfaceConfirmation == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.recordRepairSurfaceConfirmation(ctx, event.WorkspaceID, event.RunID, *event.RepairSurfaceConfirmation)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventDispatchExecutionRequested:
		if event.Executor == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.executeActiveDispatch(ctx, event.WorkspaceID, event.RunID, event.Executor)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventTerminalDeliveryAcknowledged:
		if event.DeliveryAcknowledgement == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, changed, err := c.recordDeliveryAcknowledgement(ctx, *event.DeliveryAcknowledgement)
		result.Aggregate = aggregate
		result.BudgetConsumed = changed
		return result, err
	case RuntimeInternalEventRecoveryBudgetConsumed:
		aggregate, consumed, err := c.consumeRecoveryBudget(ctx, event.WorkspaceID, event.RunID, event.RecoveryBudgetKind)
		result.Aggregate = aggregate
		result.BudgetConsumed = consumed
		return result, err
	case RuntimeInternalEventDriveLeaseScheduled:
		if event.DriveLease == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, lease, changed, err := c.scheduleDriveLease(ctx, event.WorkspaceID, event.RunID, *event.DriveLease)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventDriveLeaseClaimed:
		if event.DriveLease == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, lease, claimed, err := c.claimDriveLease(ctx, event.WorkspaceID, event.RunID, *event.DriveLease, event.LeaseNow)
		result.Aggregate, result.LeaseClaimed = aggregate, claimed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventDriveLeaseReleased:
		aggregate, changed, err := c.releaseDriveLease(ctx, event.WorkspaceID, event.RunID, event.LeaseGeneration)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		return result, err
	case RuntimeInternalEventDriveLeaseRecovered:
		aggregate, lease, changed, err := c.recoverDriveLease(ctx, event.WorkspaceID, event.RunID, event.LeaseNow)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventTerminationRequested:
		aggregate, err := c.requestTermination(ctx, event.WorkspaceID, event.RunID, event.TerminationKind, event.RequestedBy, event.Reason)
		result.Aggregate = aggregate
		return result, err
	default:
		return result, fmt.Errorf("%w: unsupported runtime internal event %q", common.ErrInvalidArgument, event.Kind)
	}
}
