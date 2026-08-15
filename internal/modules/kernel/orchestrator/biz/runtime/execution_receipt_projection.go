package runtime

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (x *activeDispatchExecution) projectOperationReceipt(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord, result RuntimeOperationExecutionResult) (activeDispatchExecutionFlow, error) {
	if err := validateOwnerReceiptIdentity(operation, execution, &result); err != nil {
		return 0, err
	}
	if result.Receipt.PrerequisiteStale != nil {
		invalidated, err := x.component.invalidateActiveDispatchForPrerequisiteStale(x.ctx, x.workspaceID, x.runID, x.dispatch.ID, result.Receipt)
		if err != nil {
			return 0, err
		}
		x.aggregate = invalidated
		return activeDispatchExecutionStop, nil
	}
	// Pre-effect governance must not enter ordinary execution repair.
	if result.Governance != nil {
		governed, err := x.component.recordOperationGovernanceRequired(x.ctx, x.workspaceID, x.runID, operation.ID, result)
		if err != nil {
			return 0, err
		}
		x.aggregate = governed
		return activeDispatchExecutionStop, nil
	}
	adjudicateRuntimeOperationResult(x.aggregate.Decision, execution, operation, &result)
	if err := validateRuntimeOperationExecutionResult(x.aggregate.Decision, operation, result); err != nil {
		return 0, err
	}
	commit, err := x.component.commitEvidence(x.ctx, executionEvidenceFromOperationResult(x.workspaceID, x.runID, x.dispatch, operation, result))
	if err != nil {
		return 0, err
	}
	x.aggregate = commit.Aggregate
	x.completed[operation.ID] = struct{}{}
	if commit.DispatchCompleted {
		return activeDispatchExecutionStop, nil
	}
	return activeDispatchExecutionReady, nil
}

func adjudicateRuntimeOperationResult(decision runtimecontract.RuntimeDecision, execution runtimecontract.RuntimeOperationExecutionRecord, operation runtimecontract.RuntimeOperation, result *RuntimeOperationExecutionResult) {
	if failure := runtimeOperationEffectMismatchFailure(decision, execution, operation, result.Receipt); failure != nil {
		result.Status = ExecutionEvidenceStatusRejected
		result.Failure = failure
	} else if failure := runtimeOperationEvidenceQualityFailure(operation, result.Receipt.Evidence); failure != nil {
		result.Status = ExecutionEvidenceStatusRejected
		result.Failure = failure
	}
}

func executionEvidenceFromOperationResult(workspaceID common.WorkspaceID, runID common.RunID, dispatch SealedDispatch, operation runtimecontract.RuntimeOperation, result RuntimeOperationExecutionResult) ExecutionEvidence {
	return ExecutionEvidence{
		EventID: result.EventID, WorkspaceID: workspaceID, RunID: runID,
		DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
		OperationID: operation.ID, Kind: result.EvidenceKind, Status: result.Status,
		SourceRef: result.SourceRef, Failure: result.Failure, Evidence: result.Evidence,
		ObservedEffectScope: result.Receipt.ObservedEffectScope, NoOp: result.Receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp,
		OwnerReceiptRef:  "runtime-receipt/" + result.Receipt.ExecutionKey,
		OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(result.Receipt), Receipt: &result.Receipt,
	}
}
