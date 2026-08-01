package orchestrator

import (
	"context"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These helpers preserve focused owner tests without restoring a production
// mutation facade. Every helper sends the same owner-local event that the Biz
// production path sends, so a test cannot bypass the dispatcher/CAS boundary.
func (o *RuntimeOrchestrator) InitializeRuntime(ctx context.Context, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext, requirements []runtime.RuntimeRequirement) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventActivationObserved, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, ActivationFacts: &facts, ActivationDefinition: &definition, ActivationRequirements: append([]runtime.RuntimeRequirement(nil), requirements...)})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) CommitRuntimePrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, prerequisite runtime.PrerequisiteResult) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventPrerequisiteObserved, WorkspaceID: workspaceID, RunID: runID, PrerequisiteResult: &prerequisite})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) InvalidateRuntimePrerequisitesForReadOnlyStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventPrerequisitesReadOnlyStaleObserved, WorkspaceID: workspaceID, RunID: runID, Stale: &stale})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) ConsumeRuntimeRecoveryBudget(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeRecoveryBudgetKind) (runtime.RuntimeAggregate, bool, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventRecoveryBudgetConsumed, WorkspaceID: workspaceID, RunID: runID, RecoveryBudgetKind: kind})
	return result.Aggregate, result.BudgetConsumed, err
}

func (o *RuntimeOrchestrator) RequestTermination(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventTerminationRequested, WorkspaceID: workspaceID, RunID: runID, TerminationKind: kind, RequestedBy: requestedBy, Reason: reason})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) EnsureRuntimeCandidateRequestSession(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stablePrefixHash, manifestHash, correctionHash string) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateRequestEnsured, WorkspaceID: workspaceID, RunID: runID, StablePrefixHash: stablePrefixHash, ManifestHash: manifestHash, CorrectionHash: correctionHash})
	return result.Aggregate, result.CandidateSession, err
}

func (o *RuntimeOrchestrator) MarkRuntimeCandidateRequestSubmitted(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, sequence int, physicalCallID string) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateRequestSubmitted, WorkspaceID: workspaceID, RunID: runID, LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) CompleteRuntimeCandidateRequestDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateDocumentObserved, WorkspaceID: workspaceID, RunID: runID, LogicalRequestID: logicalRequestID, PhysicalCallID: physicalCallID})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) BeginRuntimeCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateAttemptBegun, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, StablePrefixHash: stablePrefixHash, Manifest: &manifest, LogicalRequestID: logicalRequestID})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) AdmitRuntimeCandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateProposalObserved, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate})
	if result.Admission == nil {
		return runtime.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func (o *RuntimeOrchestrator) RecordRuntimeCandidateFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateFeasibilityObserved, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, CandidateFeasibility: &record})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) InvalidateRuntimeCandidateRequestSessionForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateRequestStaleObserved, WorkspaceID: workspaceID, RunID: runID, LogicalRequestID: logicalRequestID, Stale: &stale})
	return result.Aggregate, err
}

func (o *RuntimeOrchestrator) CommitRuntimeOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, receipt runtimecontract.RuntimeOperationReceipt) (runtime.RuntimeAggregate, runtimecontract.RuntimeOperationReceipt, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventOperationReceiptObserved, WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID, Fence: fence, OperationReceipt: &receipt})
	if result.Receipt == nil {
		return result.Aggregate, runtimecontract.RuntimeOperationReceipt{}, err
	}
	return result.Aggregate, result.Receipt.Clone(), err
}

func (o *RuntimeOrchestrator) AdmitAndSealRuntimeCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateSealed, WorkspaceID: workspaceID, RunID: runID, Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate})
	if result.Admission == nil {
		return runtime.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func (o *RuntimeOrchestrator) CommitRuntimeGovernanceResolution(ctx context.Context, resolution runtimecontract.RuntimeGovernanceResolution) (runtime.EvidenceCommitResult, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventGovernanceResolutionObserved, WorkspaceID: resolution.WorkspaceID, RunID: resolution.RunID, GovernanceResolution: &resolution})
	if result.Evidence == nil {
		return runtime.EvidenceCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Evidence, err
}

func (o *RuntimeOrchestrator) ExecuteRuntimeDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor runtime.RuntimeOperationExecutor) (runtime.RuntimeAggregate, error) {
	result, err := o.ApplyRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventDispatchExecutionRequested, WorkspaceID: workspaceID, RunID: runID, Executor: executor})
	return result.Aggregate, err
}
