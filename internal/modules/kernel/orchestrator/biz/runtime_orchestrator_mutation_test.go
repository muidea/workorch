package orchestrator

import (
	"context"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These helpers preserve focused owner tests without restoring a production
// mutation facade. Every helper sends the same owner-local event that the Biz
// production path sends, so a test cannot bypass the dispatcher/CAS boundary.
func (o *RuntimeOrchestrator) InitializeRuntimeV2(ctx context.Context, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext, requirements []runtimev2.RuntimeRequirement) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventActivationObserved, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, ActivationFacts: &facts, ActivationDefinition: &definition, ActivationRequirements: append([]runtimev2.RuntimeRequirement(nil), requirements...)})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) CommitRuntimeV2Prerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, prerequisite runtimev2.PrerequisiteResult) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventPrerequisiteObserved, WorkspaceID: workspaceID, RunID: runID, PrerequisiteResult: &prerequisite})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) InvalidateRuntimeV2PrerequisitesForReadOnlyStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stale runtimecontract.RuntimePrerequisiteStale) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventPrerequisitesReadOnlyStaleObserved, WorkspaceID: workspaceID, RunID: runID, Stale: &stale})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) ConsumeRuntimeV2RecoveryBudget(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeRecoveryBudgetKind) (runtimev2.RuntimeAggregate, bool, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventRecoveryBudgetConsumed, WorkspaceID: workspaceID, RunID: runID, RecoveryBudgetKind: kind})
	return result.Aggregate, result.BudgetConsumed, err
}

func (o *RuntimeOrchestrator) RequestTermination(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventTerminationRequested, WorkspaceID: workspaceID, RunID: runID, TerminationKind: kind, RequestedBy: requestedBy, Reason: reason})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) EnsureRuntimeV2CandidateRequestSession(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stablePrefixHash, manifestHash, correctionHash string) (runtimev2.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateRequestEnsured, WorkspaceID: workspaceID, RunID: runID, StablePrefixHash: stablePrefixHash, ManifestHash: manifestHash, CorrectionHash: correctionHash})
	return result.Aggregate, result.CandidateSession, err
}

func (o *RuntimeOrchestrator) MarkRuntimeV2CandidateRequestSubmitted(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, sequence int, physicalCallID string) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateRequestSubmitted, WorkspaceID: workspaceID, RunID: runID, LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) CompleteRuntimeV2CandidateRequestDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateDocumentObserved, WorkspaceID: workspaceID, RunID: runID, LogicalRequestID: logicalRequestID, PhysicalCallID: physicalCallID})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) BeginRuntimeV2CandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateAttemptBegun, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, StablePrefixHash: stablePrefixHash, Manifest: &manifest, LogicalRequestID: logicalRequestID})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) AdmitRuntimeV2CandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtimev2.CandidateAdmissionCommitResult, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateProposalObserved, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate})
	if result.Admission == nil {
		return runtimev2.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func (o *RuntimeOrchestrator) RecordRuntimeV2CandidateFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateFeasibilityObserved, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, CandidateFeasibility: &record})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) InvalidateRuntimeV2CandidateRequestSessionForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, stale runtimecontract.RuntimePrerequisiteStale) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateRequestStaleObserved, WorkspaceID: workspaceID, RunID: runID, LogicalRequestID: logicalRequestID, Stale: &stale})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) CommitRuntimeV2OperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, receipt runtimecontract.RuntimeOperationReceipt) (runtimev2.RuntimeAggregate, runtimecontract.RuntimeOperationReceipt, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventOperationReceiptObserved, WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID, Fence: fence, OperationReceipt: &receipt})
	if result.Receipt == nil {
		return result.Aggregate, runtimecontract.RuntimeOperationReceipt{}, err
	}
	return result.Aggregate, result.Receipt.Clone(), err
}

func (o *RuntimeOrchestrator) AdmitAndSealRuntimeV2CandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtimev2.CandidateAdmissionCommitResult, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventCandidateSealed, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate})
	if result.Admission == nil {
		return runtimev2.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func (o *RuntimeOrchestrator) CommitRuntimeV2GovernanceResolution(ctx context.Context, resolution runtimecontract.RuntimeGovernanceResolution) (runtimev2.EvidenceCommitResult, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventGovernanceResolutionObserved, WorkspaceID: resolution.WorkspaceID, RunID: resolution.RunID, GovernanceResolution: &resolution})
	if result.Evidence == nil {
		return runtimev2.EvidenceCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Evidence, err
}

func (o *RuntimeOrchestrator) ExecuteRuntimeV2Dispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor runtimev2.RuntimeOperationExecutor) (runtimev2.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{Kind: runtimev2.RuntimeInternalEventDispatchExecutionRequested, WorkspaceID: workspaceID, RunID: runID, Executor: executor})
	return result.Aggregate, err
}
