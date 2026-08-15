package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (x *activeDispatchExecution) prepareOperation(operation runtimecontract.RuntimeOperation) (preparedActiveDispatchOperation, error) {
	execution, found := operationExecutionRecord(x.aggregate.ActiveDispatch.OperationExecutions, operation.ID)
	if !found {
		_, _ = x.component.markOperationExecutionOutcomeUnknown(x.ctx, x.workspaceID, x.runID, operation.ID)
		return preparedActiveDispatchOperation{}, operationOutcomeUnknown(operation.ID, "execution ledger is missing")
	}
	switch execution.State {
	case runtimecontract.OperationExecutionEvidenceCommitted:
		x.completed[operation.ID] = struct{}{}
		return preparedActiveDispatchOperation{flow: activeDispatchExecutionSkip}, nil
	case runtimecontract.OperationExecutionOwnerCommitted:
		return x.prepareOwnerCommittedOperation(operation, execution)
	case runtimecontract.OperationExecutionClaimed:
		return x.prepareClaimedOperation(operation, execution)
	case runtimecontract.OperationExecutionOutcomeUnknown:
		return preparedActiveDispatchOperation{}, operationOutcomeUnknown(operation.ID, "blind replay is forbidden")
	case runtimecontract.OperationExecutionSealed:
		return x.claimSealedOperation(operation)
	default:
		// Aggregate validation excludes this shape. Preserve the former execution
		// coordinator fall-through so this structural split does not introduce a
		// second state classifier at the side-effect boundary.
		return preparedActiveDispatchOperation{execution: execution, flow: activeDispatchExecutionReady}, nil
	}
}

func (x *activeDispatchExecution) prepareOwnerCommittedOperation(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord) (preparedActiveDispatchOperation, error) {
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(x.aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found {
		return preparedActiveDispatchOperation{}, fmt.Errorf("operation %q owner_committed without persisted receipt", operation.ID)
	}
	return x.projectPersistedOperationReceipt(operation, receipt)
}

func (x *activeDispatchExecution) prepareClaimedOperation(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord) (preparedActiveDispatchOperation, error) {
	if receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(x.aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence); found {
		return x.projectPersistedOperationReceipt(operation, receipt)
	}
	governanceGrant := governancecheck.GrantForExecution(x.aggregate.OperationGovernanceBindings, execution)
	if governanceGrant != nil {
		return preparedActiveDispatchOperation{execution: execution, governanceGrant: governanceGrant, flow: activeDispatchExecutionReady}, nil
	}
	if err := x.authorizeClaimedOperationReplay(operation, execution); err != nil {
		return preparedActiveDispatchOperation{}, err
	}
	x.dispatch = *x.aggregate.ActiveDispatch
	execution, found := operationExecutionRecord(x.dispatch.OperationExecutions, operation.ID)
	if !found || execution.State != runtimecontract.OperationExecutionClaimed {
		return preparedActiveDispatchOperation{}, fmt.Errorf("operation %q recovery lost claimed execution", operation.ID)
	}
	return preparedActiveDispatchOperation{execution: execution, flow: activeDispatchExecutionReady}, nil
}

func (x *activeDispatchExecution) authorizeClaimedOperationReplay(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord) error {
	idempotency := runtimecontract.ResolveOperationIdempotencyClass(operation, x.aggregate.Facts.Activation.MCPCapabilities)
	hasReplayProof := execution.ReplayProof != nil
	if idempotency != runtimecontract.RuntimeOperationIdempotencySafeToReplay && idempotency != runtimecontract.RuntimeOperationIdempotencyOwnerManaged && !hasReplayProof {
		_, _ = x.component.markOperationExecutionOutcomeUnknown(x.ctx, x.workspaceID, x.runID, operation.ID)
		return operationOutcomeUnknown(operation.ID, "claimed execution has no receipt or replay authority")
	}
	// The persisted counter and operation identity form one exact SM-4 handoff
	// fence; an unrelated later charge cannot authorize this replay.
	authorized := strings.TrimSpace(x.recoveryOperationID) == operation.ID && x.recoveryBudgetAttempt > 0 && x.aggregate.Facts.Recovery.OperationRecoveryAttempts == x.recoveryBudgetAttempt
	if authorized {
		x.recoveryOperationID = ""
		x.recoveryBudgetAttempt = 0
		return nil
	}
	if !runtimeOperationRecoveryBudgetAvailable(x.aggregate) {
		_, _ = x.component.markOperationExecutionOutcomeUnknown(x.ctx, x.workspaceID, x.runID, operation.ID)
		return operationOutcomeUnknown(operation.ID, "safe replay recovery budget exhausted")
	}
	return &runtimeOperationRecoveryBudgetRequiredError{OperationID: operation.ID}
}

func (x *activeDispatchExecution) claimSealedOperation(operation runtimecontract.RuntimeOperation) (preparedActiveDispatchOperation, error) {
	claimed, err := x.component.claimOperationExecution(x.ctx, x.workspaceID, x.runID, operation.ID)
	if err != nil {
		return preparedActiveDispatchOperation{}, err
	}
	x.aggregate = claimed
	x.dispatch = *x.aggregate.ActiveDispatch
	execution, found := operationExecutionRecord(x.dispatch.OperationExecutions, operation.ID)
	if !found || execution.State != runtimecontract.OperationExecutionClaimed {
		return preparedActiveDispatchOperation{}, fmt.Errorf("operation %q claim was not persisted", operation.ID)
	}
	return preparedActiveDispatchOperation{execution: execution, flow: activeDispatchExecutionReady}, nil
}

func (x *activeDispatchExecution) projectPersistedOperationReceipt(operation runtimecontract.RuntimeOperation, receipt runtimecontract.RuntimeOperationReceipt) (preparedActiveDispatchOperation, error) {
	if receipt.PrerequisiteStale != nil {
		invalidated, err := x.component.invalidateActiveDispatchForPrerequisiteStale(x.ctx, x.workspaceID, x.runID, x.dispatch.ID, receipt)
		if err != nil {
			return preparedActiveDispatchOperation{}, err
		}
		x.aggregate = invalidated
		return preparedActiveDispatchOperation{flow: activeDispatchExecutionStop}, nil
	}
	commit, err := x.component.commitPersistedOperationReceiptEvidence(x.ctx, x.aggregate, x.dispatch, operation, receipt)
	if err != nil {
		return preparedActiveDispatchOperation{}, err
	}
	x.aggregate = commit.Aggregate
	x.completed[operation.ID] = struct{}{}
	if commit.DispatchCompleted {
		return preparedActiveDispatchOperation{flow: activeDispatchExecutionStop}, nil
	}
	return preparedActiveDispatchOperation{flow: activeDispatchExecutionSkip}, nil
}
