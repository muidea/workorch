package runtime

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These test-only adapters preserve focused reducer seams without compiling a
// second RuntimeController mutation surface into production. Production
// dispatch reaches the owning private component only through
// RuntimeController.ApplyInternalEvent and its internal-event routes.
func (c *RuntimeController) scheduleDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, requested runtimecontract.RuntimeDriveLease) (RuntimeAggregate, runtimecontract.RuntimeDriveLease, bool, error) {
	if c == nil || c.driveDelivery == nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	return c.driveDelivery.scheduleDriveLease(ctx, workspaceID, runID, requested, 0)
}

func (c *RuntimeController) claimDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, requested runtimecontract.RuntimeDriveLease, now string) (RuntimeAggregate, runtimecontract.RuntimeDriveLease, bool, error) {
	if c == nil || c.driveDelivery == nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	return c.driveDelivery.claimDriveLease(ctx, workspaceID, runID, requested, now)
}

func (c *RuntimeController) releaseDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, generation uint64) (RuntimeAggregate, bool, error) {
	if c == nil || c.driveDelivery == nil {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	return c.driveDelivery.releaseDriveLease(ctx, workspaceID, runID, generation)
}

func (c *RuntimeController) recoverDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dueAt string) (RuntimeAggregate, runtimecontract.RuntimeDriveLease, bool, error) {
	if c == nil || c.driveDelivery == nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	return c.driveDelivery.recoverDriveLease(ctx, workspaceID, runID, dueAt)
}

func (c *RuntimeController) consumeRecoveryBudget(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeRecoveryBudgetKind) (RuntimeAggregate, bool, error) {
	if c == nil || c.driveDelivery == nil {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	result, err := c.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:  RuntimeInternalEventRecoveryBudgetConsumed,
		Scope: RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
		Delivery: &RuntimeInternalDeliveryPayload{
			RecoveryBudgetKind: kind,
		},
	})
	return result.Aggregate, result.BudgetConsumed, err
}

func (c *RuntimeController) recordDeliveryAcknowledgement(ctx context.Context, acknowledgement runtimecontract.RuntimeDeliveryAcknowledgement) (RuntimeAggregate, bool, error) {
	if c == nil || c.driveDelivery == nil {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	return c.driveDelivery.recordDeliveryAcknowledgement(ctx, acknowledgement)
}

func (c *RuntimeController) commitOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, ownerModuleID string, receipt runtimecontract.RuntimeOperationReceipt) (RuntimeAggregate, runtimecontract.RuntimeOperationReceipt, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.commitOperationReceipt(ctx, workspaceID, runID, dispatchID, fence, ownerModuleID, receipt)
}

func (c *RuntimeController) reconcileAuthorityBeforeCandidate(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.reconcileAuthorityBeforeCandidate(ctx, workspaceID, runID)
}

func (c *RuntimeController) requireSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, feasibilityBatchID string, refreshGeneration int) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.requireSemanticContract(ctx, workspaceID, runID, feasibilityBatchID, refreshGeneration)
}

func (c *RuntimeController) invalidateSemanticCatalogGeneration(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifestHash, logicalRequestID string) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.invalidateSemanticCatalogGeneration(ctx, workspaceID, runID, manifestHash, logicalRequestID)
}

func (c *RuntimeController) compileSemanticPlanBatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, batch runtimecontract.RuntimeSemanticPlanBatch) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.compileSemanticPlanBatch(ctx, workspaceID, runID, batch)
}

func (c *RuntimeController) advanceSemanticPlanBatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, identity runtimecontract.RuntimeSemanticPlanBatch, next runtimecontract.RuntimeSemanticPlanBatchState) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.advanceSemanticPlanBatch(ctx, workspaceID, runID, identity, next)
}

func (c *RuntimeController) observeSemanticPlanBatchFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, result runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.observeSemanticPlanBatchFeasibility(ctx, workspaceID, runID, result)
}

func (c *RuntimeController) observeSemanticPlanBatchDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, document runtimecontract.RuntimeSemanticPlanDocument) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.observeSemanticPlanBatchDocument(ctx, workspaceID, runID, document)
}

func (c *RuntimeController) mergeSemanticPlanBatchDocuments(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, merged runtimecontract.RuntimeSemanticPlanDocument) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.mergeSemanticPlanBatchDocuments(ctx, workspaceID, runID, merged)
}

func (c *RuntimeController) mergeSemanticPlanBatchFeasibilities(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, values []runtimecontract.RuntimeSemanticProofFeasibility) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.mergeSemanticPlanBatchFeasibilities(ctx, workspaceID, runID, values)
}

func (c *RuntimeController) startSemanticPlanRequest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifest runtimecontract.RuntimeSemanticPlanManifest, stablePrefixHash, logicalRequestID string) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.startSemanticPlanRequest(ctx, workspaceID, runID, manifest, stablePrefixHash, logicalRequestID)
}

func (c *RuntimeController) evaluateSemanticPlan(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, document runtimecontract.RuntimeSemanticPlanDocument) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.evaluateSemanticPlan(ctx, workspaceID, runID, logicalRequestID, document)
}

func (c *RuntimeController) requireSemanticFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.requireSemanticFeasibility(ctx, workspaceID, runID)
}

func (c *RuntimeController) observeSemanticFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, batchID string, observation runtimecontract.RuntimeSemanticProofFeasibility) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.observeSemanticFeasibility(ctx, workspaceID, runID, batchID, observation)
}

func (c *RuntimeController) requireSemanticPlanCorrection(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stablePrefixHash, logicalRequestID string, expected runtimecontract.RuntimeExpectedDocumentContract, rejection runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.requireSemanticPlanCorrection(ctx, workspaceID, runID, stablePrefixHash, logicalRequestID, expected, rejection)
}

func (c *RuntimeController) sealSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.sealSemanticContract(ctx, workspaceID, runID)
}

func (c *RuntimeController) failSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, terminalCode string, failure runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.failSemanticContract(ctx, workspaceID, runID, terminalCode, failure)
}

func (c *RuntimeController) markCandidateRequestSubmitted(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, sequence int, physicalCallID string) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.markCandidateRequestSubmitted(ctx, workspaceID, runID, logicalRequestID, sequence, physicalCallID)
}

func (c *RuntimeController) coordinateProviderRequestRecovery(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string, fact RuntimeProviderRequestRecoveryFact, budgetSettlement ...*RuntimeProviderBudgetSettlement) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.coordinateProviderRequestRecovery(ctx, workspaceID, runID, logicalRequestID, physicalCallID, fact, budgetSettlement...)
}

func (c *RuntimeController) completeCandidateRequestDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.completeCandidateRequestDocument(ctx, workspaceID, runID, logicalRequestID, physicalCallID)
}

func (c *RuntimeController) rejectCompletedCandidateDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, rejectedCandidate *runtimecontract.RuntimeCandidate, failure runtimecontract.RuntimeCandidateFailure) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.rejectCompletedCandidateDocument(ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, logicalRequestID, rejectedCandidate, failure)
}

func (c *RuntimeController) admitCandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	if c == nil || c.candidateDispatch == nil {
		return CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.admitCandidateProposal(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func (c *RuntimeController) recordCandidateFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.recordCandidateFeasibility(ctx, workspaceID, runID, attempt, record)
}

func (c *RuntimeController) admitAndSealCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	if c == nil || c.candidateDispatch == nil {
		return CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.admitAndSealCandidateAttempt(ctx, workspaceID, runID, attempt, manifest, logicalRequestID, candidate)
}

func (c *RuntimeController) commitEvidence(ctx context.Context, evidence ExecutionEvidence) (EvidenceCommitResult, error) {
	if c == nil || c.candidateDispatch == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.commitEvidence(ctx, evidence)
}

// executeActiveDispatch is a focused Runtime test harness.
// Production dispatch enters candidateDispatchComponent through
// ApplyInternalEvent; an SM-4 recovery budget is submitted as a separate
// owner-private event by Biz. The test seam mirrors that two-event sequence
// without exposing another public mutation entry.
func (c *RuntimeController) executeActiveDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor RuntimeOperationExecutor) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil || c.driveDelivery == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	var recoveryOperationID string
	var recoveryBudgetAttempt int
	for transition := 0; transition <= runtimecontract.RuntimeRecoveryBudgetLimit; transition++ {
		result, err := c.ApplyInternalEvent(ctx, RuntimeInternalEvent{
			Kind:  RuntimeInternalEventDispatchExecutionRequested,
			Scope: RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
			Transaction: &RuntimeInternalTransactionPayload{
				Executor: executor, RecoveryOperationID: recoveryOperationID, RecoveryBudgetAttempt: recoveryBudgetAttempt,
			},
		})
		aggregate := result.Aggregate
		recoveryOperationID, recoveryBudgetAttempt = "", 0
		if operationID, required := RuntimeOperationRecoveryBudgetRequired(err); !required {
			return aggregate, err
		} else {
			// Keep the test harness on the same owner-private event route as
			// production Biz. The candidate facet only hands off; SM-4 owns the
			// budget transition through ApplyInternalEvent.
			budget, budgetErr := c.ApplyInternalEvent(ctx, RuntimeInternalEvent{
				Kind:  RuntimeInternalEventRecoveryBudgetConsumed,
				Scope: RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
				Delivery: &RuntimeInternalDeliveryPayload{
					RecoveryBudgetKind: runtimecontract.RuntimeRecoveryBudgetOperationRecovery,
				},
			})
			if budgetErr != nil {
				return RuntimeAggregate{}, budgetErr
			}
			if budget.BudgetConsumed {
				recoveryOperationID = operationID
				recoveryBudgetAttempt = budget.Aggregate.Facts.Recovery.OperationRecoveryAttempts
			}
		}
	}
	return RuntimeAggregate{}, common.ErrConflict
}

func (c *RuntimeController) sealGovernanceDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.sealGovernanceDispatch(ctx, workspaceID, runID, manifest, candidate)
}

func (c *RuntimeController) failCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, failure runtimecontract.RuntimeCandidateFailure) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.failCandidateAttempt(ctx, workspaceID, runID, attempt, failure)
}

func (c *RuntimeController) recordFeasibilityFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.RuntimeFeasibilityFact) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.recordFeasibilityFact(ctx, workspaceID, runID, fact)
}

func (c *RuntimeController) recordFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.recordFailureFact(ctx, workspaceID, runID, fact)
}

func (c *RuntimeController) recordDriveFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.recordDriveFailureFact(ctx, workspaceID, runID, fact)
}

func (c *RuntimeController) commitPrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, result PrerequisiteResult) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.commitPrerequisite(ctx, workspaceID, runID, result)
}

func (c *RuntimeController) commitAssessment(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.commitAssessment(ctx, workspaceID, runID)
}

func (c *RuntimeController) projectEligibleRuntimeFocus(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifestHash string, acceptedOrder []common.CoverageWorkItemID) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.projectEligibleRuntimeFocus(ctx, workspaceID, runID, manifestHash, acceptedOrder)
}

func (c *RuntimeController) retireRuntimeCWIBinding(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact RuntimeCWIBindingRetirementFact) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.retireRuntimeCWIBinding(ctx, workspaceID, runID, fact)
}

func (c *RuntimeController) recordValidationFailureObservation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, observation runtimecontract.RuntimeValidationFailureObservation) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.recordValidationFailureObservation(ctx, workspaceID, runID, observation)
}

func (c *RuntimeController) recordRepairSurfaceConfirmation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.recordRepairSurfaceConfirmation(ctx, workspaceID, runID, confirmation)
}

func (c *RuntimeController) bindGovernanceRequest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, request runtimecontract.RuntimeGovernanceRequestRecord) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.bindGovernanceRequest(ctx, workspaceID, runID, request)
}

func (c *RuntimeController) commitGovernanceResolution(ctx context.Context, resolution runtimecontract.RuntimeGovernanceResolution) (EvidenceCommitResult, error) {
	if c == nil || c.authority == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	return c.authority.commitGovernanceResolution(ctx, resolution)
}

func (c *RuntimeController) requestTermination(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (RuntimeAggregate, error) {
	if c == nil || c.driveDelivery == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.driveDelivery.requestTermination(ctx, workspaceID, runID, kind, requestedBy, reason)
}

func (c *RuntimeController) beginCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.beginCandidateAttempt(ctx, workspaceID, runID, attempt, stablePrefixHash, manifest, logicalRequestID)
}

func (c *RuntimeController) ensureCandidateRequestSessionWithExpected(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, documentKind runtimecontract.RuntimeProviderDocumentKind, stablePrefixHash, manifestHash, correctionHash string, expected *runtimecontract.RuntimeExpectedDocumentContract) (RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	if c == nil || c.planning == nil || c.authority == nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, common.ErrInvalidArgument
	}
	ensured, err := c.planning.ensureCandidateRequestSessionWithExpected(ctx, workspaceID, runID, documentKind, stablePrefixHash, manifestHash, correctionHash, expected)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	if ensured.AuthorityFailure != nil {
		terminal, terminalErr := c.ApplyInternalEvent(ctx, RuntimeInternalEvent{
			Kind:  RuntimeInternalEventFailureObserved,
			Scope: RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
			Authority: &RuntimeInternalAuthorityPayload{
				FailureFact: ensured.AuthorityFailure,
			},
		})
		return terminal.Aggregate, runtimecontract.RuntimeCandidateRequestSession{}, terminalErr
	}
	return ensured.Aggregate, ensured.Session, nil
}

func (c *RuntimeController) invalidateSemanticCatalogForFocusScope(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, gap RuntimeSemanticCatalogScopeGap) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.invalidateSemanticCatalogForFocusScope(ctx, workspaceID, runID, gap)
}

func (c *RuntimeController) invalidatePrerequisitesForReadOnlyStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stale runtimecontract.RuntimePrerequisiteStale) (RuntimeAggregate, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.authority.invalidatePrerequisitesForReadOnlyStale(ctx, workspaceID, runID, stale)
}

func (c *RuntimeController) invalidateCandidateRequestSessionForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, stale runtimecontract.RuntimePrerequisiteStale) (RuntimeAggregate, error) {
	if c == nil || c.planning == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.planning.invalidateCandidateRequestSessionForPrerequisiteStale(ctx, workspaceID, runID, logicalRequestID, stale)
}

func (c *RuntimeController) invalidateCandidateAttemptForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stale runtimecontract.RuntimePrerequisiteStale) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.invalidateCandidateAttemptForPrerequisiteStale(ctx, workspaceID, runID, attempt, stale)
}

func (c *RuntimeController) invalidateCandidateAttemptForFeasibilityPrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, request runtimecontract.RuntimeOperationPrerequisiteRequest) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.invalidateCandidateAttemptForFeasibilityPrerequisite(ctx, workspaceID, runID, attempt, request)
}

func (c *RuntimeController) invalidateCandidateAttemptForOwnerInfeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.invalidateCandidateAttemptForOwnerInfeasibility(ctx, workspaceID, runID, attempt, observation)
}

func (c *RuntimeController) invalidateActiveDispatchForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, receipt runtimecontract.RuntimeOperationReceipt) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.invalidateActiveDispatchForPrerequisiteStale(ctx, workspaceID, runID, dispatchID, receipt)
}

func (c *RuntimeController) claimOperationExecution(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID string) (RuntimeAggregate, error) {
	if c == nil || c.candidateDispatch == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.candidateDispatch.claimOperationExecution(ctx, workspaceID, runID, operationID)
}

func (c *RuntimeController) ensureReconciliationRequest(ctx context.Context, request runtimecontract.RuntimeCWIReconciliationRequest) (RuntimeAggregate, bool, error) {
	if c == nil || c.driveDelivery == nil {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	return c.driveDelivery.ensureReconciliationRequest(ctx, request)
}

func (c *RuntimeController) commitReconciliationBatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, batch runtimecontract.RuntimeCWIReconciliationBatch) (RuntimeAggregate, bool, error) {
	if c == nil || c.authority == nil {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	return c.authority.commitReconciliationBatch(ctx, workspaceID, runID, batch)
}
