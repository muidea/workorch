package orchestrator

import (
	"context"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These fixtures intentionally expose no production mutation API. Parent
// package crash/restart tests submit the same concrete owner-private fact as
// production and can still replace the repository beneath the controller to
// exercise exact CAS boundaries.
func runtimeTestControllerInitialize(controller *runtime.RuntimeController, ctx context.Context, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext, requirements []runtime.RuntimeRequirement) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventActivationObserved, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, ActivationFacts: &facts, ActivationDefinition: &definition, ActivationRequirements: append([]runtime.RuntimeRequirement(nil), requirements...)})
	return result.Aggregate, err
}

func runtimeTestControllerCommitPrerequisite(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, prerequisite runtime.PrerequisiteResult) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventPrerequisiteObserved, WorkspaceID: workspaceID, RunID: runID, PrerequisiteResult: &prerequisite})
	return result.Aggregate, err
}

func runtimeTestControllerInvalidateReadOnlyStale(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventPrerequisitesReadOnlyStaleObserved, WorkspaceID: workspaceID, RunID: runID, Stale: &stale})
	return result.Aggregate, err
}

func runtimeTestControllerRecordFailure(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, failure runtimecontract.FailureFact) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventFailureObserved, WorkspaceID: workspaceID, RunID: runID, FailureFact: &failure})
	return result.Aggregate, err
}

func runtimeTestControllerEnsureCandidateRequest(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, documentKind runtimecontract.RuntimeProviderDocumentKind, stablePrefixHash, manifestHash, correctionHash string) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateRequestEnsured, WorkspaceID: workspaceID, RunID: runID, DocumentKind: documentKind, StablePrefixHash: stablePrefixHash, ManifestHash: manifestHash, CorrectionHash: correctionHash})
	return result.Aggregate, result.CandidateSession, err
}

func runtimeTestControllerMarkCandidateRequestSubmitted(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, sequence int, physicalCallID string) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateRequestSubmitted, WorkspaceID: workspaceID, RunID: runID, LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID})
	return result.Aggregate, err
}

func runtimeTestControllerCompleteCandidateDocument(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateDocumentObserved, WorkspaceID: workspaceID, RunID: runID, LogicalRequestID: logicalRequestID, PhysicalCallID: physicalCallID})
	return result.Aggregate, err
}

func runtimeTestControllerBeginCandidateAttempt(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateAttemptBegun, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, StablePrefixHash: stablePrefixHash, Manifest: &manifest, LogicalRequestID: logicalRequestID})
	return result.Aggregate, err
}

func runtimeTestControllerAdmitCandidateProposal(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateProposalObserved, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate})
	if result.Admission == nil {
		return runtime.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func runtimeTestControllerRecordCandidateFeasibility(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, feasibility runtimecontract.RuntimeCandidateFeasibilityRecord) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateFeasibilityObserved, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, CandidateFeasibility: &feasibility})
	return result.Aggregate, err
}

func runtimeTestControllerInvalidateCandidateStale(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidatePrerequisiteStaleObserved, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, Stale: &stale})
	return result.Aggregate, err
}

func runtimeTestControllerAdmitAndSealCandidate(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateSealed, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate})
	if result.Admission == nil {
		return runtime.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func runtimeTestControllerSealGovernanceDispatch(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventGovernanceDispatchSealed, WorkspaceID: workspaceID, RunID: runID, Manifest: &manifest, Candidate: &candidate})
	return result.Aggregate, err
}

func runtimeTestControllerCommitGovernanceResolution(controller *runtime.RuntimeController, ctx context.Context, resolution runtimecontract.RuntimeGovernanceResolution) (runtime.EvidenceCommitResult, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventGovernanceResolutionObserved, WorkspaceID: resolution.WorkspaceID, RunID: resolution.RunID, GovernanceResolution: &resolution})
	if result.Evidence == nil {
		return runtime.EvidenceCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Evidence, err
}

func runtimeTestControllerCommitOperationReceipt(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, receipt runtimecontract.RuntimeOperationReceipt) (runtime.RuntimeAggregate, runtimecontract.RuntimeOperationReceipt, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventOperationReceiptObserved, WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID, Fence: fence, OperationReceipt: &receipt})
	if result.Receipt == nil {
		return result.Aggregate, runtimecontract.RuntimeOperationReceipt{}, err
	}
	return result.Aggregate, result.Receipt.Clone(), err
}

func runtimeTestControllerExecuteDispatch(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor runtime.RuntimeOperationExecutor) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventDispatchExecutionRequested, WorkspaceID: workspaceID, RunID: runID, Executor: executor})
	return result.Aggregate, err
}

func runtimeTestControllerRequestTermination(controller *runtime.RuntimeController, ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventTerminationRequested, WorkspaceID: workspaceID, RunID: runID, TerminationKind: kind, RequestedBy: requestedBy, Reason: reason})
	return result.Aggregate, err
}
