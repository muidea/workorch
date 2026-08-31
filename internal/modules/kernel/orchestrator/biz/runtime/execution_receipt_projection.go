package runtime

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (x *activeDispatchExecution) projectOperationReceipt(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord, result RuntimeOperationExecutionResult) (activeDispatchExecutionFlow, *RuntimeDispatchExecutionHandoff, error) {
	if err := validateOwnerReceiptIdentity(operation, execution, &result); err != nil {
		return 0, nil, err
	}
	if result.Receipt.PrerequisiteStale != nil {
		receipt := result.Receipt.Clone()
		return activeDispatchExecutionStop, &RuntimeDispatchExecutionHandoff{
			DispatchID: x.dispatch.ID, OperationID: operation.ID, PrerequisiteStaleReceipt: &receipt,
		}, nil
	}
	// Pre-effect governance must not enter ordinary execution repair.
	if result.Governance != nil {
		value := result
		return activeDispatchExecutionStop, &RuntimeDispatchExecutionHandoff{
			DispatchID: x.dispatch.ID, OperationID: operation.ID, GovernanceRequired: &value,
		}, nil
	}
	if err := adjudicateRuntimeOperationResult(x.aggregate.Decision, execution, operation, &result); err != nil {
		return 0, nil, err
	}
	if err := validateRuntimeOperationExecutionResult(x.aggregate.Decision, operation, result); err != nil {
		return 0, nil, err
	}
	evidence := executionEvidenceFromOperationResult(x.workspaceID, x.runID, x.dispatch, operation, result)
	return activeDispatchExecutionStop, &RuntimeDispatchExecutionHandoff{
		DispatchID: x.dispatch.ID, OperationID: operation.ID, Evidence: &evidence,
	}, nil
}

func adjudicateRuntimeOperationResult(decision runtimecontract.RuntimeDecision, execution runtimecontract.RuntimeOperationExecutionRecord, operation runtimecontract.RuntimeOperation, result *RuntimeOperationExecutionResult) error {
	if result == nil {
		return common.ErrInvalidArgument
	}
	status, failure, err := runtimeOperationReceiptEvidenceJudgment(decision, execution, operation, result.Receipt)
	if err != nil {
		return err
	}
	result.Status = status
	result.Failure = failure
	return nil
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
