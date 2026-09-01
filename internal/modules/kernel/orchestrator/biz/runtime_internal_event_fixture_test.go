package orchestrator

import (
	"context"
	"fmt"
	"time"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These helpers preserve focused owner tests without restoring a production
// mutation facade. Every helper sends the same owner-local event that the Biz
// production path sends, so a test cannot bypass the dispatcher/CAS boundary.
func (o *Biz) InitializeRuntime(ctx context.Context, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext, requirements []runtimecontract.RuntimeRequirement) (runtime.RuntimeAggregate, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventActivationObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID}, Activation: &runtime.RuntimeInternalActivationPayload{ActivationFacts: &facts, ActivationDefinition: &definition, ActivationRequirements: append([]runtimecontract.RuntimeRequirement(nil), requirements...)}})
	return result.Aggregate, err
}

func (o *Biz) CommitRuntimePrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, prerequisite runtime.PrerequisiteResult) (runtime.RuntimeAggregate, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventPrerequisiteObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Authority: &runtime.RuntimeInternalAuthorityPayload{PrerequisiteResult: &prerequisite}})
	return result.Aggregate, err
}

func (o *Biz) InvalidateRuntimePrerequisitesForReadOnlyStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventPrerequisitesReadOnlyStaleObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Authority: &runtime.RuntimeInternalAuthorityPayload{Stale: &stale}})
	return result.Aggregate, err
}

func (o *Biz) ConsumeRuntimeRecoveryBudget(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeRecoveryBudgetKind) (runtime.RuntimeAggregate, bool, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventRecoveryBudgetConsumed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoveryBudgetKind: kind}})
	return result.Aggregate, result.BudgetConsumed, err
}

func (o *Biz) RequestTermination(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (runtime.RuntimeAggregate, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventTerminationRequested, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{TerminationKind: kind, RequestedBy: requestedBy, Reason: reason}})
	return result.Aggregate, err
}

func (o *Biz) EnsureRuntimeCandidateRequestSession(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stablePrefixHash, manifestHash, correctionHash string) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession, error) {
	aggregate, err := o.loadRuntime(ctx, workspaceID, runID)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	attempt := 0
	if correctionHash != "" {
		attempt = 1
	}
	manifest, err := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	expected, err := runtimeOperationsExpectedDocument(aggregate, manifest, attempt)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidateRequestSession{}, err
	}
	// Stale-identity fixtures may intentionally supply an opaque manifest hash;
	// the real current wire hash remains sealed so the test changes one identity
	// dimension at a time.
	expected.ManifestHash = manifestHash
	if expected.MaxOutputTokens <= 0 {
		expected.MaxOutputTokens = 8192
	}
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidateRequestEnsured, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, StablePrefixHash: stablePrefixHash,
			ManifestHash: manifestHash, CorrectionHash: correctionHash, ExpectedDocument: &expected},
	})
	return result.Aggregate, result.CandidateSession, err
}

func (o *Biz) MarkRuntimeCandidateRequestSubmitted(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, sequence int, physicalCallID string) (runtime.RuntimeAggregate, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateRequestSubmitted, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID}})
	return result.Aggregate, err
}

func (o *Biz) CompleteRuntimeCandidateRequestDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID, physicalCallID string) (runtime.RuntimeAggregate, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateDocumentObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{LogicalRequestID: logicalRequestID, PhysicalCallID: physicalCallID}})
	return result.Aggregate, err
}

func (o *Biz) BeginRuntimeCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) (runtime.RuntimeAggregate, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateAttemptBegun, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, StablePrefixHash: stablePrefixHash, Manifest: &manifest, LogicalRequestID: logicalRequestID}})
	return result.Aggregate, err
}

func (o *Biz) AdmitRuntimeCandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateProposalObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate}})
	if result.Admission == nil {
		return runtime.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func (o *Biz) RecordRuntimeCandidateFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (runtime.RuntimeAggregate, error) {
	if record.CandidateManifestHash == "" {
		aggregate, err := o.loadRuntime(ctx, workspaceID, runID)
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
		_, pending, found := pendingRuntimeCandidateAttemptForTest(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
		if !found || pending.ManifestHash == "" {
			return runtime.RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d has no pending manifest identity", common.ErrConflict, attempt)
		}
		record.CandidateManifestHash = pending.ManifestHash
	}
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateFeasibilityObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, CandidateFeasibility: &record}})
	return result.Aggregate, err
}

func (o *Biz) InvalidateRuntimeCandidateRequestSessionForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, stale runtimecontract.RuntimePrerequisiteStale) (runtime.RuntimeAggregate, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateRequestStaleObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{LogicalRequestID: logicalRequestID, Stale: &stale}})
	return result.Aggregate, err
}

func (o *Biz) CommitRuntimeOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, ownerSource string, receipt runtimecontract.RuntimeOperationReceipt) (runtime.RuntimeAggregate, runtimecontract.RuntimeOperationReceipt, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventOperationReceiptObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{DispatchID: dispatchID, Fence: fence, OperationReceiptOwnerSource: ownerSource, OperationReceipt: &receipt}})
	if result.Receipt == nil {
		return result.Aggregate, runtimecontract.RuntimeOperationReceipt{}, err
	}
	return result.Aggregate, result.Receipt.Clone(), err
}

func (o *Biz) AdmitAndSealRuntimeCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (runtime.CandidateAdmissionCommitResult, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventCandidateSealed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: attempt, Manifest: &manifest, LogicalRequestID: logicalRequestID, Candidate: &candidate}})
	if result.Admission == nil {
		return runtime.CandidateAdmissionCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Admission, err
}

func (o *Biz) CommitRuntimeGovernanceResolution(ctx context.Context, resolution runtimecontract.RuntimeGovernanceResolution) (runtime.EvidenceCommitResult, error) {
	result, err := o.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventGovernanceResolutionObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: resolution.WorkspaceID, RunID: resolution.RunID}, Transaction: runtimeGovernanceResolutionTransactionForTest(resolution)})
	if result.Evidence == nil {
		return runtime.EvidenceCommitResult{Aggregate: result.Aggregate}, err
	}
	return *result.Evidence, err
}

func (o *Biz) ExecuteRuntimeDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor runtime.RuntimeOperationExecutor) (runtime.RuntimeAggregate, error) {
	return executeRuntimeDispatchWithExecutor(ctx, workspaceID, runID, executor, o.submitRuntimeInternalEvent)
}

func executeRuntimeDispatchWithExecutor(
	ctx context.Context,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	executor runtime.RuntimeOperationExecutor,
	submit func(context.Context, runtime.RuntimeInternalEvent) (runtime.RuntimeInternalEventResult, error),
) (runtime.RuntimeAggregate, error) {
	if executor == nil || submit == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	recoveryClaimID := ""
	var operationCallErr error
	maxTransitions := runtimecontract.RuntimeCandidateOperationMaxCount + runtimecontract.RuntimeRecoveryBudgetLimit*3 + 4
	for transition := 0; transition <= maxTransitions; transition++ {
		result, err := submit(ctx, runtime.RuntimeInternalEvent{
			Kind:        runtime.RuntimeInternalEventDispatchExecutionRequested,
			Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
			Transaction: &runtime.RuntimeInternalTransactionPayload{RecoveryClaimID: recoveryClaimID, RecoveryClaimNow: time.Now().UTC().Format(time.RFC3339Nano)},
		})
		recoveryClaimID = ""
		if operationID, authorizationID, claimRequired := runtime.RuntimeOperationRecoveryAuthorizationClaimRequired(err); claimRequired {
			claimed, claimErr := submit(ctx, runtime.RuntimeInternalEvent{
				Kind: runtime.RuntimeInternalEventOperationRecoveryClaimed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
				Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoveryOperationID: operationID, RecoveryAuthorizationID: authorizationID, LeaseNow: time.Now().UTC().Format(time.RFC3339Nano)},
			})
			if claimErr != nil {
				return claimed.Aggregate, claimErr
			}
			if !claimed.RecoveryClaimChanged || claimed.RecoveryClaimID == "" {
				return claimed.Aggregate, common.ErrConflict
			}
			recoveryClaimID = claimed.RecoveryClaimID
			continue
		}
		if operationID, required := runtime.RuntimeOperationOutcomeUnknownTransitionRequired(err); required {
			observed, observeErr := submit(ctx, runtime.RuntimeInternalEvent{
				Kind: runtime.RuntimeInternalEventOperationExecutionOutcomeUnknownObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
				Transaction: &runtime.RuntimeInternalTransactionPayload{OperationOutcomeUnknownID: operationID},
			})
			if observeErr != nil {
				return observed.Aggregate, observeErr
			}
			if operationCallErr != nil {
				return observed.Aggregate, operationCallErr
			}
			continue
		}
		if err == nil && result.OperationCall != nil {
			if result.OperationCall.Validate() != nil {
				return result.Aggregate, common.ErrConflict
			}
			call := result.OperationCall
			_, operationCallErr = executor.ExecuteRuntimeOperation(ctx, call.Request)
			if call.RecoveryClaimID != "" {
				returned, returnErr := submit(ctx, runtime.RuntimeInternalEvent{
					Kind: runtime.RuntimeInternalEventOperationRecoveryCallReturned, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
					Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoveryOperationID: call.Request.Operation.ID,
						RecoveryAuthorizationID: call.RecoveryAuthorizationID, RecoveryClaimID: call.RecoveryClaimID},
				})
				if returnErr != nil {
					return returned.Aggregate, returnErr
				}
			}
			continue
		}
		if err == nil && result.DispatchExecution != nil {
			handoff := result.DispatchExecution
			if handoff.Validate() != nil {
				return result.Aggregate, common.ErrConflict
			}
			operationCallErr = nil
			event := runtime.RuntimeInternalEvent{Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Transaction: &runtime.RuntimeInternalTransactionPayload{}}
			switch {
			case handoff.Evidence != nil:
				event.Kind = runtime.RuntimeInternalEventOperationEvidenceObserved
				event.Transaction.ExecutionEvidence = handoff.Evidence
			case handoff.PrerequisiteStaleReceipt != nil:
				event.Kind = runtime.RuntimeInternalEventOperationPrerequisiteStaleObserved
				event.Transaction.DispatchID = handoff.DispatchID
				event.Transaction.OperationPrerequisiteStaleReceipt = handoff.PrerequisiteStaleReceipt
			case handoff.GovernanceRequired != nil:
				event.Kind = runtime.RuntimeInternalEventOperationGovernanceRequiredObserved
				event.Transaction.OperationGovernanceResult = handoff.GovernanceRequired
			default:
				return result.Aggregate, common.ErrConflict
			}
			committed, commitErr := submit(ctx, event)
			if commitErr != nil {
				return committed.Aggregate, commitErr
			}
			if committed.Aggregate.ActiveDispatch == nil {
				return committed.Aggregate, nil
			}
			continue
		}
		if operationID, required := runtime.RuntimeOperationRecoveryBudgetRequired(err); required {
			budget, budgetErr := submit(ctx, runtime.RuntimeInternalEvent{
				Kind:     runtime.RuntimeInternalEventRecoveryBudgetConsumed,
				Scope:    runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
				Delivery: &runtime.RuntimeInternalDeliveryPayload{RecoveryBudgetKind: runtimecontract.RuntimeRecoveryBudgetOperationRecovery, RecoveryOperationID: operationID, LeaseNow: time.Now().UTC().Format(time.RFC3339Nano)},
			})
			if budgetErr != nil {
				return budget.Aggregate, budgetErr
			}
			continue
		}
		return result.Aggregate, err
	}
	return runtime.RuntimeAggregate{}, common.ErrConflict
}
