package runtime

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

// applyTransactionInternalEvent keeps the transaction payload family behind
// one private route, then delegates to cohesive planning, candidate/dispatch,
// or governance handlers. No delegate owns persistence or another entry point.
func (c *RuntimeController) applyTransactionInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	switch event.Kind {
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
		RuntimeInternalEventProviderRequestTerminalCoordinated:
		return c.applyPlanningTransactionInternalEvent(ctx, event)
	case RuntimeInternalEventCandidateAttemptBegun,
		RuntimeInternalEventCandidateDocumentRejected,
		RuntimeInternalEventCandidateProposalObserved,
		RuntimeInternalEventCandidateFeasibilityObserved,
		RuntimeInternalEventCandidatePrerequisiteStaleObserved,
		RuntimeInternalEventCandidatePrerequisiteObserved,
		RuntimeInternalEventCandidateSealed,
		RuntimeInternalEventGovernanceDispatchSealed,
		RuntimeInternalEventCandidateRejected,
		RuntimeInternalEventOperationReceiptObserved,
		RuntimeInternalEventDispatchExecutionRequested,
		RuntimeInternalEventOperationEvidenceObserved,
		RuntimeInternalEventOperationPrerequisiteStaleObserved,
		RuntimeInternalEventOperationGovernanceRequiredObserved,
		RuntimeInternalEventOperationExecutionOutcomeUnknownObserved,
		RuntimeInternalEventDefinitionClarificationDispatchSealed,
		RuntimeInternalEventDefinitionClarificationRequestObserved:
		return c.applyCandidateDispatchTransactionInternalEvent(ctx, event)
	case RuntimeInternalEventGovernanceResolutionObserved:
		return c.applyGovernanceResolutionTransactionInternalEvent(ctx, event)
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}

func (c *RuntimeController) applyPlanningTransactionInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	result := RuntimeInternalEventResult{}
	switch event.Kind {
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
	case RuntimeInternalEventProviderRequestRecoveryCoordinated:
		if event.Transaction.ProviderRequestRecovery == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.coordinateProviderRequestRecovery(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, event.Transaction.PhysicalCallID, *event.Transaction.ProviderRequestRecovery, event.Transaction.ProviderBudgetSettlement)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateDocumentObserved:
		var aggregate RuntimeAggregate
		var err error
		if event.Transaction.ProviderDocumentFraming != nil {
			aggregate, err = c.planning.rejectCandidateRequestDocumentConformance(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, event.Transaction.PhysicalCallID, *event.Transaction.ProviderDocumentFraming, event.Transaction.ProviderBudgetSettlement)
		} else {
			aggregate, err = c.planning.completeCandidateRequestDocument(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, event.Transaction.PhysicalCallID, event.Transaction.ProviderBudgetSettlement)
		}
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCandidateRequestStaleObserved:
		if event.Transaction.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.invalidateCandidateRequestSessionForPrerequisiteStale(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, *event.Transaction.Stale, event.Transaction.ProviderBudgetSettlement)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventProviderRequestTerminalCoordinated:
		if event.Transaction.ProviderRequestTerminal == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.coordinateProviderRequestTerminal(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.LogicalRequestID, *event.Transaction.ProviderRequestTerminal, event.Transaction.ProviderBudgetSettlement)
		result.Aggregate = aggregate
		return result, err
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}

func (c *RuntimeController) applyCandidateDispatchTransactionInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	result := RuntimeInternalEventResult{}
	switch event.Kind {
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
		aggregate, err := c.candidateDispatch.rejectCompletedCandidateDocument(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.Attempt, event.Transaction.StablePrefixHash, *event.Transaction.Manifest, event.Transaction.LogicalRequestID, event.Transaction.Candidate, *event.Transaction.CandidateFailure)
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
	case RuntimeInternalEventOperationReceiptObserved:
		if event.Transaction.OperationReceipt == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, receipt, err := c.candidateDispatch.commitOperationReceipt(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.DispatchID, event.Transaction.Fence, event.Transaction.OperationReceiptOwnerSource, *event.Transaction.OperationReceipt)
		result.Aggregate = aggregate
		result.Receipt = &receipt
		return result, err
	case RuntimeInternalEventDispatchExecutionRequested:
		aggregate, call, handoff, err := c.candidateDispatch.executeActiveDispatch(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.RecoveryClaimID, event.Transaction.RecoveryClaimNow)
		result.Aggregate = aggregate
		result.OperationCall = call
		result.DispatchExecution = handoff
		return result, err
	case RuntimeInternalEventOperationEvidenceObserved:
		if event.Transaction.ExecutionEvidence == nil {
			return result, common.ErrInvalidArgument
		}
		commit, err := c.candidateDispatch.commitEvidence(ctx, *event.Transaction.ExecutionEvidence)
		result.Aggregate = commit.Aggregate
		result.Evidence = &commit
		return result, err
	case RuntimeInternalEventOperationPrerequisiteStaleObserved:
		if event.Transaction.OperationPrerequisiteStaleReceipt == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.invalidateActiveDispatchForPrerequisiteStale(
			ctx,
			event.Scope.WorkspaceID,
			event.Scope.RunID,
			event.Transaction.DispatchID,
			*event.Transaction.OperationPrerequisiteStaleReceipt,
		)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventOperationGovernanceRequiredObserved:
		if event.Transaction.OperationGovernanceResult == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.recordOperationGovernanceRequired(
			ctx,
			event.Scope.WorkspaceID,
			event.Scope.RunID,
			event.Transaction.OperationGovernanceResult.OperationID,
			*event.Transaction.OperationGovernanceResult,
		)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventOperationExecutionOutcomeUnknownObserved:
		if event.Transaction.OperationOutcomeUnknownID == "" {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.markOperationExecutionOutcomeUnknown(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Transaction.OperationOutcomeUnknownID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventDefinitionClarificationDispatchSealed:
		aggregate, err := c.candidateDispatch.sealRuntimeDefinitionClarificationDispatch(ctx, event.Scope.WorkspaceID, event.Scope.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventDefinitionClarificationRequestObserved:
		if event.Transaction.DefinitionClarificationRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.bindRuntimeDefinitionClarificationRequest(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Transaction.DefinitionClarificationRequest)
		result.Aggregate = aggregate
		return result, err
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}

func (c *RuntimeController) applyGovernanceResolutionTransactionInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	if event.Kind != RuntimeInternalEventGovernanceResolutionObserved || event.Transaction.GovernanceResolution == nil {
		return RuntimeInternalEventResult{}, common.ErrInvalidArgument
	}
	commit, err := c.authority.commitGovernanceResolutionWithContinuationLease(
		ctx,
		*event.Transaction.GovernanceResolution,
		event.Transaction.GovernanceContinuationDriveLease,
	)
	result := RuntimeInternalEventResult{
		Aggregate: commit.Aggregate,
		Evidence:  &commit,
	}
	if requested := event.Transaction.GovernanceContinuationDriveLease; requested != nil && commit.Aggregate.DriveLease != nil &&
		commit.Aggregate.DriveLease.WorkspaceID == requested.WorkspaceID && commit.Aggregate.DriveLease.RunID == requested.RunID &&
		commit.Aggregate.DriveLease.Reason == requested.Reason && commit.Aggregate.DriveLease.FailureSignature == requested.FailureSignature {
		lease := commit.Aggregate.DriveLease.Clone()
		result.DriveLease = &lease
		result.LeaseChanged = commit.Disposition == CommitDispositionCommitted
	}
	return result, err
}
