package runtimev2

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
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
	// bootstrap fact for the one-time Runtime V2 aggregate creation.  They are
	// deliberately carried through the same private dispatcher as every later
	// mutation; a Run notification never calls a controller reducer directly.
	ActivationFacts        *runtimecontract.RuntimeFacts
	ActivationDefinition   *runtimecontract.RuntimeDefinitionContext
	ActivationRequirements []RuntimeRequirement

	ReconciliationRequest *runtimecontract.RuntimeCWIReconciliationRequest
	ReconciliationBatch   *runtimecontract.RuntimeCWIReconciliationBatch

	PrerequisiteResult  *PrerequisiteResult
	Stale               *runtimecontract.RuntimePrerequisiteStale
	FeasibilityFact     *runtimecontract.RuntimeFeasibilityFact
	FailureFact         *runtimecontract.FailureFact
	CloseActiveDispatch bool

	StablePrefixHash string
	ManifestHash     string
	CorrectionHash   string
	LogicalRequestID string
	Sequence         int
	PhysicalCallID   string
	RequestFragment  *runtimecontract.RuntimeCandidateRequestFragment
	RequestOutcome   *runtimecontract.RuntimeCandidateRequestOutcome

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

const (
	RuntimeInternalEventActivationObserved                 RuntimeInternalEventKind = "activation_observed"
	RuntimeInternalEventReconcileAuthority                 RuntimeInternalEventKind = "reconcile_authority"
	RuntimeInternalEventReconciliationRequested            RuntimeInternalEventKind = "reconciliation_requested"
	RuntimeInternalEventReconciliationBatchObserved        RuntimeInternalEventKind = "reconciliation_batch_observed"
	RuntimeInternalEventPrerequisiteObserved               RuntimeInternalEventKind = "prerequisite_observed"
	RuntimeInternalEventPrerequisitesReadOnlyStaleObserved RuntimeInternalEventKind = "prerequisites_read_only_stale_observed"
	RuntimeInternalEventFeasibilityObserved                RuntimeInternalEventKind = "feasibility_observed"
	RuntimeInternalEventFailureObserved                    RuntimeInternalEventKind = "failure_observed"
	RuntimeInternalEventDriveFailureObserved               RuntimeInternalEventKind = "drive_failure_observed"
	RuntimeInternalEventCandidateRequestEnsured            RuntimeInternalEventKind = "candidate_request_ensured"
	RuntimeInternalEventCandidateRequestSubmitted          RuntimeInternalEventKind = "candidate_request_submitted"
	RuntimeInternalEventCandidateRequestFragmentObserved   RuntimeInternalEventKind = "candidate_request_fragment_observed"
	RuntimeInternalEventCandidateRequestOutcomeObserved    RuntimeInternalEventKind = "candidate_request_outcome_observed"
	RuntimeInternalEventCandidateDocumentObserved          RuntimeInternalEventKind = "candidate_document_observed"
	RuntimeInternalEventCandidateRequestStaleObserved      RuntimeInternalEventKind = "candidate_request_stale_observed"
	RuntimeInternalEventCandidateRequestTerminated         RuntimeInternalEventKind = "candidate_request_terminated"
	RuntimeInternalEventCandidateAttemptBegun              RuntimeInternalEventKind = "candidate_attempt_begun"
	RuntimeInternalEventCandidateDocumentRejected          RuntimeInternalEventKind = "candidate_document_rejected"
	RuntimeInternalEventCandidateProposalObserved          RuntimeInternalEventKind = "candidate_proposal_observed"
	RuntimeInternalEventCandidateFeasibilityObserved       RuntimeInternalEventKind = "candidate_feasibility_observed"
	RuntimeInternalEventCandidatePrerequisiteStaleObserved RuntimeInternalEventKind = "candidate_prerequisite_stale_observed"
	RuntimeInternalEventCandidatePrerequisiteObserved      RuntimeInternalEventKind = "candidate_prerequisite_observed"
	RuntimeInternalEventOwnerFeasibilityObserved           RuntimeInternalEventKind = "owner_feasibility_observed"
	RuntimeInternalEventCandidateSealed                    RuntimeInternalEventKind = "candidate_sealed"
	RuntimeInternalEventGovernanceDispatchSealed           RuntimeInternalEventKind = "governance_dispatch_sealed"
	RuntimeInternalEventCandidateRejected                  RuntimeInternalEventKind = "candidate_rejected"
	RuntimeInternalEventAssessmentObserved                 RuntimeInternalEventKind = "assessment_observed"
	RuntimeInternalEventGovernanceRequestObserved          RuntimeInternalEventKind = "governance_request_observed"
	RuntimeInternalEventGovernanceResolutionObserved       RuntimeInternalEventKind = "governance_resolution_observed"
	RuntimeInternalEventOperationReceiptObserved           RuntimeInternalEventKind = "operation_receipt_observed"
	RuntimeInternalEventValidationFailureObserved          RuntimeInternalEventKind = "validation_failure_observed"
	RuntimeInternalEventRepairSurfaceConfirmationObserved  RuntimeInternalEventKind = "repair_surface_confirmation_observed"
	RuntimeInternalEventDispatchExecutionRequested         RuntimeInternalEventKind = "dispatch_execution_requested"
	RuntimeInternalEventTerminalDeliveryAcknowledged       RuntimeInternalEventKind = "terminal_delivery_ack_observed"
	RuntimeInternalEventRecoveryBudgetConsumed             RuntimeInternalEventKind = "recovery_budget_consumed"
	RuntimeInternalEventDriveLeaseScheduled                RuntimeInternalEventKind = "drive_lease_scheduled"
	RuntimeInternalEventDriveLeaseClaimed                  RuntimeInternalEventKind = "drive_lease_claimed"
	RuntimeInternalEventDriveLeaseReleased                 RuntimeInternalEventKind = "drive_lease_released"
	RuntimeInternalEventDriveLeaseRecovered                RuntimeInternalEventKind = "drive_lease_recovered"
	RuntimeInternalEventTerminationRequested               RuntimeInternalEventKind = "termination_requested"
)

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
	result := RuntimeInternalEventResult{}
	switch event.Kind {
	case RuntimeInternalEventActivationObserved:
		if event.ActivationFacts == nil || event.ActivationDefinition == nil ||
			event.ActivationFacts.WorkspaceID != event.WorkspaceID || event.ActivationFacts.RunID != event.RunID {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.Initialize(ctx, *event.ActivationFacts, event.ActivationDefinition.Clone(), append([]RuntimeRequirement(nil), event.ActivationRequirements...))
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconcileAuthority:
		aggregate, err := c.ReconcileAuthorityBeforeCandidate(ctx, event.WorkspaceID, event.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconciliationRequested:
		if event.ReconciliationRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, _, err := c.EnsureReconciliationRequest(ctx, *event.ReconciliationRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconciliationBatchObserved:
		if event.ReconciliationBatch == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, _, err := c.CommitReconciliationBatch(ctx, event.WorkspaceID, event.RunID, *event.ReconciliationBatch)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventPrerequisiteObserved:
		if event.PrerequisiteResult == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.CommitPrerequisite(ctx, event.WorkspaceID, event.RunID, *event.PrerequisiteResult)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventPrerequisitesReadOnlyStaleObserved:
		if event.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.InvalidatePrerequisitesForReadOnlyStale(ctx, event.WorkspaceID, event.RunID, *event.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventFeasibilityObserved:
		if event.FeasibilityFact == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.RecordFeasibilityFact(ctx, event.WorkspaceID, event.RunID, *event.FeasibilityFact)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventFailureObserved, RuntimeInternalEventDriveFailureObserved:
		if event.FailureFact == nil {
			return result, common.ErrInvalidArgument
		}
		var aggregate RuntimeAggregate
		var err error
		if event.Kind == RuntimeInternalEventDriveFailureObserved || event.CloseActiveDispatch {
			aggregate, err = c.RecordDriveFailureFact(ctx, event.WorkspaceID, event.RunID, *event.FailureFact)
		} else {
			aggregate, err = c.RecordFailureFact(ctx, event.WorkspaceID, event.RunID, *event.FailureFact)
		}
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestEnsured:
		aggregate, session, err := c.EnsureCandidateRequestSession(ctx, event.WorkspaceID, event.RunID, event.StablePrefixHash, event.ManifestHash, event.CorrectionHash)
		result.Aggregate = aggregate
		result.CandidateSession = session
		return result, err
	case RuntimeInternalEventCandidateRequestSubmitted:
		aggregate, err := c.MarkCandidateRequestSubmitted(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, event.Sequence, event.PhysicalCallID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestFragmentObserved:
		if event.RequestFragment == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.RecordCandidateRequestFragment(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, *event.RequestFragment)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestOutcomeObserved:
		if event.RequestOutcome == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.RecordCandidateRequestTransportOutcome(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, event.PhysicalCallID, *event.RequestOutcome)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateDocumentObserved:
		aggregate, err := c.CompleteCandidateRequestDocument(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, event.PhysicalCallID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestStaleObserved:
		if event.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.InvalidateCandidateRequestSessionForPrerequisiteStale(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, *event.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestTerminated:
		if event.RequestOutcome == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.TerminateCandidateRequestSession(ctx, event.WorkspaceID, event.RunID, event.LogicalRequestID, *event.RequestOutcome)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateAttemptBegun:
		if event.Manifest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.BeginCandidateAttempt(ctx, event.WorkspaceID, event.RunID, event.Attempt, event.StablePrefixHash, *event.Manifest, event.LogicalRequestID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateDocumentRejected:
		if event.Manifest == nil || event.CandidateFailure == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.RejectCompletedCandidateDocument(ctx, event.WorkspaceID, event.RunID, event.Attempt, event.StablePrefixHash, *event.Manifest, event.LogicalRequestID, *event.CandidateFailure)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateProposalObserved:
		if event.Manifest == nil || event.Candidate == nil {
			return result, common.ErrInvalidArgument
		}
		admission, err := c.AdmitCandidateProposal(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.Manifest, event.LogicalRequestID, *event.Candidate)
		result.Aggregate = admission.Aggregate
		result.Admission = &admission
		return result, err
	case RuntimeInternalEventCandidateFeasibilityObserved:
		if event.CandidateFeasibility == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.RecordCandidateFeasibility(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.CandidateFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidatePrerequisiteStaleObserved:
		if event.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.InvalidateCandidateAttemptForPrerequisiteStale(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidatePrerequisiteObserved:
		if event.PrerequisiteRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.InvalidateCandidateAttemptForFeasibilityPrerequisite(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.PrerequisiteRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventOwnerFeasibilityObserved:
		if event.OwnerFeasibility == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.InvalidateCandidateAttemptForOwnerInfeasibility(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.OwnerFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateSealed:
		if event.Manifest == nil || event.Candidate == nil {
			return result, common.ErrInvalidArgument
		}
		admission, err := c.AdmitAndSealCandidateAttempt(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.Manifest, event.LogicalRequestID, *event.Candidate)
		result.Aggregate = admission.Aggregate
		result.Admission = &admission
		return result, err
	case RuntimeInternalEventGovernanceDispatchSealed:
		if event.Manifest == nil || event.Candidate == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.SealGovernanceDispatch(ctx, event.WorkspaceID, event.RunID, *event.Manifest, *event.Candidate)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRejected:
		if event.CandidateFailure == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.FailCandidateAttempt(ctx, event.WorkspaceID, event.RunID, event.Attempt, *event.CandidateFailure)
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
		aggregate, err := c.BindGovernanceRequest(ctx, event.WorkspaceID, event.RunID, *event.GovernanceRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventGovernanceResolutionObserved:
		if event.GovernanceResolution == nil {
			return result, common.ErrInvalidArgument
		}
		commit, err := c.CommitGovernanceResolution(ctx, *event.GovernanceResolution)
		result.Aggregate = commit.Aggregate
		result.Evidence = &commit
		return result, err
	case RuntimeInternalEventOperationReceiptObserved:
		if event.OperationReceipt == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, receipt, err := c.CommitOperationReceipt(ctx, event.WorkspaceID, event.RunID, event.DispatchID, event.Fence, *event.OperationReceipt)
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
		aggregate, err := c.RecordValidationFailureObservation(ctx, event.WorkspaceID, event.RunID, *event.FailureFact.ValidationFailureObservation)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventRepairSurfaceConfirmationObserved:
		if event.RepairSurfaceConfirmation == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.RecordRepairSurfaceConfirmation(ctx, event.WorkspaceID, event.RunID, *event.RepairSurfaceConfirmation)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventDispatchExecutionRequested:
		if event.Executor == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.ExecuteActiveDispatch(ctx, event.WorkspaceID, event.RunID, event.Executor)
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
		aggregate, consumed, err := c.ConsumeRecoveryBudget(ctx, event.WorkspaceID, event.RunID, event.RecoveryBudgetKind)
		result.Aggregate = aggregate
		result.BudgetConsumed = consumed
		return result, err
	case RuntimeInternalEventDriveLeaseScheduled:
		if event.DriveLease == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, lease, changed, err := c.ScheduleDriveLease(ctx, event.WorkspaceID, event.RunID, *event.DriveLease)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventDriveLeaseClaimed:
		if event.DriveLease == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, lease, claimed, err := c.ClaimDriveLease(ctx, event.WorkspaceID, event.RunID, *event.DriveLease, event.LeaseNow)
		result.Aggregate, result.LeaseClaimed = aggregate, claimed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventDriveLeaseReleased:
		aggregate, changed, err := c.ReleaseDriveLease(ctx, event.WorkspaceID, event.RunID, event.LeaseGeneration)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		return result, err
	case RuntimeInternalEventDriveLeaseRecovered:
		aggregate, lease, changed, err := c.RecoverDriveLease(ctx, event.WorkspaceID, event.RunID, event.LeaseNow)
		result.Aggregate, result.LeaseChanged = aggregate, changed
		if lease.WorkspaceID != "" {
			result.DriveLease = &lease
		}
		return result, err
	case RuntimeInternalEventTerminationRequested:
		aggregate, err := c.RequestTermination(ctx, event.WorkspaceID, event.RunID, event.TerminationKind, event.RequestedBy, event.Reason)
		result.Aggregate = aggregate
		return result, err
	default:
		return result, fmt.Errorf("%w: unsupported runtime internal event %q", common.ErrInvalidArgument, event.Kind)
	}
}
