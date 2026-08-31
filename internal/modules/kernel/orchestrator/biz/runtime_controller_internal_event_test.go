package orchestrator

import (
	"context"
	"time"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These fixtures intentionally expose no production mutation API. Parent
// package crash/restart tests submit the same concrete owner-private fact as
// production and can still replace the repository beneath the controller to
// exercise exact CAS boundaries.
func runtimeTestControllerInitialize(controller *runtime.RuntimeController, ctx context.Context, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext, requirements []runtimecontract.RuntimeRequirement) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventActivationObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID}, Activation: &runtime.RuntimeInternalActivationPayload{ActivationFacts: &facts, ActivationDefinition: &definition, ActivationRequirements: append([]runtimecontract.RuntimeRequirement(nil), requirements...)}})
	return result.Aggregate, err
}

func runtimeTestControllerCommitPrerequisite(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, prerequisite runtime.PrerequisiteResult) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventPrerequisiteObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Authority: &runtime.RuntimeInternalAuthorityPayload{PrerequisiteResult: &prerequisite}})
	return result.Aggregate, err
}

func runtimeTestControllerInvalidateReadOnlyStale(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventPrerequisitesReadOnlyStaleObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Authority: &runtime.RuntimeInternalAuthorityPayload{Stale: &stale}})
	return result.Aggregate, err
}

func runtimeTestControllerRecordFailure(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, failure runtimecontract.FailureFact) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventFailureObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Authority: &runtime.RuntimeInternalAuthorityPayload{FailureFact: &failure}})
	return result.Aggregate, err
}

func runtimeTestControllerEnsureCandidateRequest(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, documentKind runtimecontract.RuntimeProviderDocumentKind, stablePrefixHash, manifestHash, correctionHash string) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	if documentKind != runtimecontract.RuntimeProviderDocumentOperations {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, common.ErrInvalidArgument
	}
	aggregate, err := controller.Load(ctx, workspaceID, runID)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	correctionAttempt := 0
	if correctionHash != "" {
		correctionAttempt = 1
	}
	expected := runtimeOperationsExpectedDocument(aggregate, manifestHash, correctionAttempt)
	if expected.MaxOutputTokens <= 0 {
		expected.MaxOutputTokens = 8192
	}
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidateRequestEnsured, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{DocumentKind: documentKind, StablePrefixHash: stablePrefixHash, ManifestHash: manifestHash,
			CorrectionHash: correctionHash, ExpectedDocument: &expected},
	})
	return result.Aggregate, result.CandidateSession, err
}

func runtimeTestControllerEnsureCandidateRequestWithExpected(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stablePrefixHash, manifestHash, correctionHash string, expected runtimecontract.RuntimeExpectedDocumentContract) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidateRequestEnsured, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan, StablePrefixHash: stablePrefixHash,
			ManifestHash: manifestHash, CorrectionHash: correctionHash, ExpectedDocument: &expected},
	})
	return result.Aggregate, result.CandidateSession, err
}

func runtimeTestControllerMarkCandidateRequestSubmitted(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, sequence int, physicalCallID string) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateRequestSubmitted, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID}})
	return result.Aggregate, err
}

func runtimeTestControllerCompleteCandidateDocument(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateDocumentObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{LogicalRequestID: logicalRequestID, PhysicalCallID: physicalCallID}})
	return result.Aggregate, err
}

func runtimeTestControllerBeginCandidateAttempt(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateAttemptBegun, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, StablePrefixHash: stablePrefixHash, Manifest: &manifest, LogicalRequestID: logicalRequestID}})
	return result.Aggregate, err
}

func runtimeTestControllerAdmitCandidateProposal(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateProposalObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate}})
	if result.Admission == nil {
		return runtime.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func runtimeTestControllerRecordCandidateFeasibility(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, feasibility runtimecontract.RuntimeCandidateFeasibilityRecord) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateFeasibilityObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, CandidateFeasibility: &feasibility}})
	return result.Aggregate, err
}

func runtimeTestControllerInvalidateCandidateStale(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidatePrerequisiteStaleObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, Stale: &stale}})
	return result.Aggregate, err
}

func runtimeTestControllerAdmitAndSealCandidate(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateSealed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate}})
	if result.Admission == nil {
		return runtime.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func runtimeTestControllerSealGovernanceDispatch(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventGovernanceDispatchSealed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Manifest: &manifest, Candidate: &candidate}})
	return result.Aggregate, err
}

func runtimeTestControllerCommitGovernanceResolution(controller *runtime.RuntimeController, ctx context.Context, resolution runtimecontract.RuntimeGovernanceResolution) (runtime.EvidenceCommitResult, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventGovernanceResolutionObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: resolution.WorkspaceID, RunID: resolution.RunID}, Transaction: runtimeGovernanceResolutionTransactionForTest(resolution)})
	if result.Evidence == nil {
		return runtime.EvidenceCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Evidence, err
}

func runtimeGovernanceResolutionTransactionForTest(resolution runtimecontract.RuntimeGovernanceResolution) *runtime.RuntimeInternalTransactionPayload {
	transaction := &runtime.RuntimeInternalTransactionPayload{GovernanceResolution: &resolution}
	if resolution.Outcome != runtimecontract.RuntimeGovernanceResolutionOutcomeContinue && resolution.Outcome != runtimecontract.RuntimeGovernanceResolutionOutcomeFail {
		return transaction
	}
	signature, err := runtime.RuntimeGovernanceResolutionDriveLeaseSignature(resolution)
	if err != nil {
		return transaction
	}
	transaction.GovernanceContinuationDriveLease = &runtimecontract.RuntimeDriveLease{
		WorkspaceID:      resolution.WorkspaceID,
		RunID:            resolution.RunID,
		Reason:           runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry,
		FailureSignature: signature,
		DueAt:            time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano),
	}
	return transaction
}

func runtimeTestControllerCommitOperationReceipt(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, ownerSource string, receipt runtimecontract.RuntimeOperationReceipt) (runtime.RuntimeAggregate, runtimecontract.RuntimeOperationReceipt, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventOperationReceiptObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{DispatchID: dispatchID, Fence: fence, OperationReceiptOwnerSource: ownerSource, OperationReceipt: &receipt}})
	if result.Receipt == nil {
		return result.Aggregate, runtimecontract.RuntimeOperationReceipt{}, err
	}
	return result.Aggregate, result.Receipt.Clone(), err
}

func runtimeTestControllerExecuteDispatch(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor runtime.RuntimeOperationExecutor) (runtime.RuntimeAggregate, error) {
	return executeRuntimeDispatchWithExecutor(ctx, workspaceID, runID, executor, controller.ApplyInternalEvent)
}

func runtimeTestControllerRequestTermination(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventTerminationRequested, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{TerminationKind: kind, RequestedBy: requestedBy, Reason: reason}})
	return result.Aggregate, err
}
